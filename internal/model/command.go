package model

import (
	"encoding/json"
	"time"
)

// Command는 command-topic 토픽에서 수신하는 BE→Agent 명령이다. spec §5.1.
//
// Spec 필드는 job_type별 구조가 다르므로 json.RawMessage로 보존한 뒤,
// 호출 측이 JobType을 확인한 후 ScriptJobSpec 또는 LogJobSpec으로
// 재차 Unmarshal한다.
//
// 명령 만료 처리 (spec §5.1): Agent가 명령을 consume했을 때 현재 시각이
// ValidUntil을 지났으면 silent skip. 이번 단계에선 구조체만 정의하고,
// 만료 판정 로직은 후속 단계(internal/job)에서 추가한다.
type Command struct {
	ExecutionID   string          `json:"execution_id"`
	ScheduleID    string          `json:"schedule_id"`
	JobID         string          `json:"job_id"`
	TargetAgentID string          `json:"target_agent_id"`
	JobType       JobType         `json:"job_type"`
	IssuedAt      time.Time       `json:"issued_at"`
	ValidUntil    time.Time       `json:"valid_until"`
	Spec          json.RawMessage `json:"spec"`
}

// ScriptJobSpec은 Command.Spec이 SCRIPT_JOB일 때의 구조다. spec §5.1.1.
type ScriptJobSpec struct {
	ScriptPath     string   `json:"script_path"`
	Args           []string `json:"args"`
	TimeoutSeconds int      `json:"timeout_seconds"`
	OutputCapBytes int      `json:"output_cap_bytes"`
}

// LogJobSpec은 Command.Spec이 LOG_JOB일 때의 구조다. spec §5.1.2.
type LogJobSpec struct {
	LogPath  string `json:"log_path"`
	Pattern  string `json:"pattern"`
	Encoding string `json:"encoding"`
}
