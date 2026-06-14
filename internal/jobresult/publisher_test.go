package jobresult

import (
	"context"
	"errors"
	"testing"

	kgo "github.com/segmentio/kafka-go"

	"monitoring/script-agent/internal/model"
)

// fakeWriter는 발행된 토픽/키/헤더를 캡처하는 messageWriter 구현이다.
type fakeWriter struct {
	calls []capturedWrite
	err   error // 설정 시 WriteMessage가 항상 err 반환
}

type capturedWrite struct {
	topic   string
	key     string
	payload []byte
	headers []kgo.Header
}

func (w *fakeWriter) WriteMessage(ctx context.Context, topic, key string, payload []byte, headers []kgo.Header) error {
	if w.err != nil {
		return w.err
	}
	w.calls = append(w.calls, capturedWrite{topic: topic, key: key, payload: payload, headers: headers})
	return nil
}

const (
	testJobTopic = "result-topic-job"
	testLogTopic = "result-topic-log"
)

// TestPublish_RoutesByJobType: R-C 분기 정확성 — SCRIPT_JOB은 result-topic-job,
// LOG_JOB은 result-topic-log로 발행되는지(오분류 0) 검증한다.
func TestPublish_RoutesByJobType(t *testing.T) {
	cases := []struct {
		name      string
		jobType   model.JobType
		wantTopic string
	}{
		{"SCRIPT_JOB → result-topic-job", model.JobTypeScript, testJobTopic},
		{"LOG_JOB → result-topic-log", model.JobTypeLog, testLogTopic},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			fw := &fakeWriter{}
			p := NewPublisher(fw, testJobTopic, testLogTopic)

			result := model.JobResult{
				ExecutionID: "exec-1",
				AgentID:     "agent-42",
				JobType:     tc.jobType,
				Status:      model.JobStatusSuccess,
			}
			if err := p.Publish(context.Background(), result); err != nil {
				t.Fatalf("Publish 실패: %v", err)
			}

			if len(fw.calls) != 1 {
				t.Fatalf("WriteMessage 호출 수 = %d, want 1", len(fw.calls))
			}
			got := fw.calls[0]
			if got.topic != tc.wantTopic {
				t.Errorf("발행 토픽 = %q, want %q (오분류)", got.topic, tc.wantTopic)
			}
			// key=agent_id 불변(분리 무관 불변식)
			if got.key != "agent-42" {
				t.Errorf("key = %q, want %q (agent_id 기반 ordering)", got.key, "agent-42")
			}
			// envelope x-source 헤더 발행 유지 확인
			if v, ok := headerValue(got.headers, model.HeaderSource); !ok || v != model.SourceAgent {
				t.Errorf("x-source 헤더 = (%q, present=%v), want (%q, true)", v, ok, model.SourceAgent)
			}
		})
	}
}

// TestPublish_UnknownJobTypeReturnsError: 알 수 없는 job_type은 fallback 없이
// 에러를 반환해야 한다(Codex 보완2 — 오분류 방지, dispatcher fail-fast 경로).
func TestPublish_UnknownJobTypeReturnsError(t *testing.T) {
	fw := &fakeWriter{}
	p := NewPublisher(fw, testJobTopic, testLogTopic)

	result := model.JobResult{AgentID: "agent-42", JobType: model.JobType("BOGUS_JOB")}
	err := p.Publish(context.Background(), result)
	if err == nil {
		t.Fatal("unknown job_type인데 에러가 nil — 오분류 위험")
	}
	if len(fw.calls) != 0 {
		t.Errorf("unknown job_type인데 발행됨: %d건 (발행 금지여야)", len(fw.calls))
	}
}

// TestPublish_WriteErrorPropagates: writer 발행 실패가 그대로 전파되는지 확인한다
// (dispatcher가 fail-fast로 commit하지 않도록).
func TestPublish_WriteErrorPropagates(t *testing.T) {
	wantErr := errors.New("broker down")
	fw := &fakeWriter{err: wantErr}
	p := NewPublisher(fw, testJobTopic, testLogTopic)

	result := model.JobResult{AgentID: "agent-42", JobType: model.JobTypeScript}
	if err := p.Publish(context.Background(), result); err == nil {
		t.Fatal("writer 실패인데 Publish가 nil 반환")
	}
}

func headerValue(headers []kgo.Header, key string) (string, bool) {
	for _, h := range headers {
		if h.Key == key {
			return string(h.Value), true
		}
	}
	return "", false
}
