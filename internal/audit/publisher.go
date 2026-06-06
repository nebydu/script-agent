// Package audit는 audit-topic 토픽으로 발행하는 감사 이벤트
// (spec §5.3) 생성과 발행을 담당한다.
//
// 데모 단계 audit 액션 3종:
//   - AGENT_STARTED: 시작 직후 (BE 등록 역할 겸함, spec §3.2)
//   - AGENT_STOPPED: 정상 종료 직전
//   - JOB_EXECUTED: Job 실행 종료 시
package audit

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"runtime"
	"time"

	"github.com/google/uuid"

	"monitoring/script-agent/internal/kafka"
	"monitoring/script-agent/internal/model"
)

// Publisher는 audit-topic 토픽 발행자다.
// Agent 단위 정보(agent_id, version, hostname, os)는 생성 시점에 캐시.
type Publisher struct {
	writer       *kafka.Writer
	topic        string
	agentID      string
	agentVersion string
	hostname     string
	osLabel      string
}

// NewPublisher는 Publisher를 만든다. hostname / OS는 프로세스 생애 동안
// 고정으로 본다 (재시작 없이 hostname이 바뀌는 경우는 데모 범위 외).
func NewPublisher(writer *kafka.Writer, topic, agentID, agentVersion string) *Publisher {
	hostname, _ := os.Hostname()
	if hostname == "" {
		hostname = "unknown"
	}
	return &Publisher{
		writer:       writer,
		topic:        topic,
		agentID:      agentID,
		agentVersion: agentVersion,
		hostname:     hostname,
		osLabel:      runtime.GOOS + "/" + runtime.GOARCH,
	}
}

// AgentStarted는 AGENT_STARTED 이벤트를 발행한다 (spec §5.3.1).
// metadata에 hostname/os/agent_version/started_at을 포함해 BE가 Agent 목록
// 맵에 등록할 수 있게 한다 (spec §3.2).
func (p *Publisher) AgentStarted(ctx context.Context) error {
	return p.publish(ctx, p.buildAgentStarted(time.Now().UTC()))
}

// AgentStopped는 AGENT_STOPPED 이벤트를 발행한다 (spec §5.3.2).
// reason은 종료 사유(예: 수신한 signal 이름).
func (p *Publisher) AgentStopped(ctx context.Context, reason string) error {
	return p.publish(ctx, p.buildAgentStopped(time.Now().UTC(), reason))
}

// JobExecuted는 JOB_EXECUTED 이벤트를 발행한다 (spec §5.3.3).
// occurred_at은 spec 규약상 Job 종료 시각(result.FinishedAt).
// target은 호출 측이 SCRIPT/LOG_FILE 분기를 채워서 넘긴다.
func (p *Publisher) JobExecuted(ctx context.Context, result model.JobResult, target model.Target) error {
	return p.publish(ctx, p.buildJobExecuted(result, target))
}

// 이하 build* 함수는 단위 테스트 대상.

func (p *Publisher) buildAgentStarted(now time.Time) model.AuditEvent {
	return model.AuditEvent{
		EventID:    uuid.NewString(),
		Actor:      model.Actor{Type: model.ActorTypeAgent, ID: p.agentID},
		Action:     model.AuditActionAgentStarted,
		Target:     model.Target{Type: model.TargetTypeAgent, ID: p.agentID},
		Result:     model.EventResultSuccess,
		OccurredAt: now,
		Metadata: map[string]any{
			"hostname":      p.hostname,
			"os":            p.osLabel,
			"agent_version": p.agentVersion,
			"started_at":    now.Format(time.RFC3339),
		},
	}
}

func (p *Publisher) buildAgentStopped(now time.Time, reason string) model.AuditEvent {
	return model.AuditEvent{
		EventID:    uuid.NewString(),
		Actor:      model.Actor{Type: model.ActorTypeAgent, ID: p.agentID},
		Action:     model.AuditActionAgentStopped,
		Target:     model.Target{Type: model.TargetTypeAgent, ID: p.agentID},
		Result:     model.EventResultSuccess,
		OccurredAt: now,
		Metadata: map[string]any{
			"reason": reason,
		},
	}
}

func (p *Publisher) buildJobExecuted(result model.JobResult, target model.Target) model.AuditEvent {
	md := map[string]any{
		"execution_id": result.ExecutionID,
		"schedule_id":  result.ScheduleID,
		"job_id":       result.JobID,
		"job_type":     string(result.JobType),
	}
	// SCRIPT_JOB 결과면 exit_code도 노출 (spec §5.3.3 예시).
	if result.Script != nil {
		md["exit_code"] = result.Script.ExitCode
	}
	return model.AuditEvent{
		EventID:    uuid.NewString(),
		Actor:      model.Actor{Type: model.ActorTypeAgent, ID: p.agentID},
		Action:     model.AuditActionJobExecuted,
		Target:     target,
		Result:     model.EventResult(result.Status),
		OccurredAt: result.FinishedAt.UTC(),
		Metadata:   md,
	}
}

func (p *Publisher) publish(ctx context.Context, event model.AuditEvent) error {
	payload, err := json.Marshal(event)
	if err != nil {
		return fmt.Errorf("audit: marshal event: %w", err)
	}
	headers := kafka.BuildHeaders(kafka.NewMessageID(), "")
	if err := p.writer.WriteMessage(ctx, p.topic, p.agentID, payload, headers); err != nil {
		return fmt.Errorf("audit: write %s event: %w", event.Action, err)
	}
	return nil
}
