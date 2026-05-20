package model

import "time"

// JobResult는 job-results 토픽으로 발행하는 Agent→BE 결과다. spec §5.2.
//
// Script / Log 필드는 pointer로 둬서 nil이면 JSON에서 null로
// 직렬화된다 (spec 예시 5.2.1 / 5.2.2 형식 일치). SCRIPT_JOB이면
// Script만 채우고 Log는 nil, LOG_JOB이면 그 반대.
//
// StartedAt / FinishedAt은 Agent의 작업 시간이다. LOG_JOB의 경우 로그
// 라인 자체의 발생 시각과는 별개이며, 데모 단계에서는 로그 발생 시각을
// 추출하지 않는다 (spec §5.2.3 노트, ADR 10).
type JobResult struct {
	ExecutionID string        `json:"execution_id"`
	ScheduleID  string        `json:"schedule_id"`
	JobID       string        `json:"job_id"`
	AgentID     string        `json:"agent_id"`
	JobType     JobType       `json:"job_type"`
	Status      JobStatus     `json:"status"`
	StartedAt   time.Time     `json:"started_at"`
	FinishedAt  time.Time     `json:"finished_at"`
	Script      *ScriptResult `json:"script"`
	Log         *LogResult    `json:"log"`
}

// ScriptResult는 SCRIPT_JOB 결과 페이로드의 script 필드다. spec §5.2.1.
type ScriptResult struct {
	ExitCode  int    `json:"exit_code"`
	StdoutCap string `json:"stdout_cap"`
	StderrCap string `json:"stderr_cap"`
	Truncated bool   `json:"truncated"`
}

// LogResult는 LOG_JOB 결과 페이로드의 log 필드다. spec §5.2.2.
type LogResult struct {
	MatchedLinesCount int      `json:"matched_lines_count"`
	SampleLines       []string `json:"sample_lines"`
}
