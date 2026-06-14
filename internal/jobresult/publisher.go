// Package jobresult는 JobResult를 job_type별 결과 토픽으로 분기 발행한다
// (T4-2 result-topic 분리): SCRIPT_JOB→result-topic-job, LOG_JOB→result-topic-log.
// audit과 의미가 달라 별도 패키지로 둔다.
package jobresult

import (
	"context"
	"encoding/json"
	"fmt"

	kgo "github.com/segmentio/kafka-go"

	"monitoring/script-agent/internal/kafka"
	"monitoring/script-agent/internal/model"
)

// messageWriter는 Publisher가 의존하는 최소 발행 인터페이스다.
// concrete *kafka.Writer가 이를 충족하며, 테스트에서 토픽 캡처용 fake를
// 주입하기 위한 seam이다.
type messageWriter interface {
	WriteMessage(ctx context.Context, topic, key string, payload []byte, headers []kgo.Header) error
}

// Publisher는 job_type별 결과 토픽 발행자다. 단일 writer가 두 토픽을 처리한다.
type Publisher struct {
	writer   messageWriter
	jobTopic string // SCRIPT_JOB 결과 토픽
	logTopic string // LOG_JOB 결과 토픽
}

func NewPublisher(writer messageWriter, jobTopic, logTopic string) *Publisher {
	return &Publisher{writer: writer, jobTopic: jobTopic, logTopic: logTopic}
}

// topicFor는 job_type에 따라 발행 토픽을 고른다. 알 수 없는 job_type은
// 오분류 방지를 위해 fallback 없이 에러를 반환한다 — dispatcher의 fail-fast
// 경로(commit 금지/redeliver)로 흘러간다.
func (p *Publisher) topicFor(jt model.JobType) (string, error) {
	switch jt {
	case model.JobTypeScript:
		return p.jobTopic, nil
	case model.JobTypeLog:
		return p.logTopic, nil
	default:
		return "", fmt.Errorf("jobresult: unknown job_type %q", jt)
	}
}

// Publish는 JobResult를 JSON으로 직렬화해 job_type에 맞는 토픽으로 발행한다.
// key는 result.AgentID (spec §2.3 — agent_id 기반 ordering).
func (p *Publisher) Publish(ctx context.Context, result model.JobResult) error {
	topic, err := p.topicFor(result.JobType)
	if err != nil {
		return err
	}
	payload, err := json.Marshal(result)
	if err != nil {
		return fmt.Errorf("jobresult: marshal: %w", err)
	}
	headers := kafka.BuildHeaders(kafka.NewMessageID(), "")
	if err := p.writer.WriteMessage(ctx, topic, result.AgentID, payload, headers); err != nil {
		return fmt.Errorf("jobresult: write: %w", err)
	}
	return nil
}
