package model

import "time"

// AuditEvent는 audit-topic 토픽으로 발행하는 감사 이벤트다. spec §5.3.
//
// Metadata는 action별 자유 형식이므로 map[string]any로 둔다.
// JOB_EXECUTED의 OccurredAt은 Job 실행 종료 시각(= JobResult.FinishedAt)으로
// 한다 (spec §5.3).
type AuditEvent struct {
	EventID    string         `json:"event_id"`
	Actor      Actor          `json:"actor"`
	Action     AuditAction    `json:"action"`
	Target     Target         `json:"target"`
	Result     EventResult    `json:"result"`
	OccurredAt time.Time      `json:"occurred_at"`
	Metadata   map[string]any `json:"metadata,omitempty"`
}

// Actor는 감사 이벤트 발행 주체. 데모 단계는 Type=AGENT 고정. spec §5.3.4.
type Actor struct {
	Type ActorType `json:"type"`
	ID   string    `json:"id"`
}

// Target은 감사 이벤트 대상. SCRIPT_JOB이면 Type=SCRIPT + ID=script_path,
// LOG_JOB이면 Type=LOG_FILE + ID=log_path, AGENT_* 이벤트는 Type=AGENT +
// ID=agent_id. spec §5.3.4.
type Target struct {
	Type TargetType `json:"type"`
	ID   string     `json:"id"`
}
