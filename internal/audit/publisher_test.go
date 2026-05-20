package audit

import (
	"encoding/json"
	"testing"
	"time"

	"monitoring/script-agent/internal/model"
)

func fixedPublisher() *Publisher {
	// kafka writer 없이 build* 만 검증하므로 nil OK.
	return &Publisher{
		agentID:      "agent-001",
		agentVersion: "0.1.0",
		hostname:     "demo-host-01",
		osLabel:      "linux/amd64",
	}
}

// TestBuildAgentStarted_SpecFormat: spec §5.3.1 예시 형식과 일치하는지.
func TestBuildAgentStarted_SpecFormat(t *testing.T) {
	p := fixedPublisher()
	fixed := time.Date(2026, 5, 19, 13, 55, 0, 0, time.UTC)
	e := p.buildAgentStarted(fixed)

	if e.Action != model.AuditActionAgentStarted {
		t.Errorf("Action = %q", e.Action)
	}
	if e.Actor.Type != model.ActorTypeAgent || e.Actor.ID != "agent-001" {
		t.Errorf("Actor = %+v", e.Actor)
	}
	if e.Target.Type != model.TargetTypeAgent || e.Target.ID != "agent-001" {
		t.Errorf("Target = %+v", e.Target)
	}
	if e.Result != model.EventResultSuccess {
		t.Errorf("Result = %q", e.Result)
	}
	if !e.OccurredAt.Equal(fixed) {
		t.Errorf("OccurredAt = %v", e.OccurredAt)
	}

	// JSON으로 가서 metadata 키 정합 확인.
	data, err := json.Marshal(e)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var raw map[string]any
	if err := json.Unmarshal(data, &raw); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	md, _ := raw["metadata"].(map[string]any)
	if md["hostname"] != "demo-host-01" {
		t.Errorf("metadata.hostname = %v", md["hostname"])
	}
	if md["os"] != "linux/amd64" {
		t.Errorf("metadata.os = %v", md["os"])
	}
	if md["agent_version"] != "0.1.0" {
		t.Errorf("metadata.agent_version = %v", md["agent_version"])
	}
	if md["started_at"] != "2026-05-19T13:55:00Z" {
		t.Errorf("metadata.started_at = %v", md["started_at"])
	}
}

// TestBuildAgentStopped_SpecFormat: spec §5.3.2 — metadata.reason 포함.
func TestBuildAgentStopped_SpecFormat(t *testing.T) {
	p := fixedPublisher()
	fixed := time.Date(2026, 5, 19, 18, 30, 0, 0, time.UTC)
	e := p.buildAgentStopped(fixed, "SIGTERM")

	if e.Action != model.AuditActionAgentStopped {
		t.Errorf("Action = %q", e.Action)
	}
	if e.Result != model.EventResultSuccess {
		t.Errorf("Result = %q", e.Result)
	}
	if reason, _ := e.Metadata["reason"].(string); reason != "SIGTERM" {
		t.Errorf("metadata.reason = %v", e.Metadata["reason"])
	}
}

// TestBuildJobExecuted_ScriptJob: spec §5.3.3 — SCRIPT_JOB이면
// target.type=SCRIPT, metadata.exit_code 포함, occurred_at=FinishedAt.
func TestBuildJobExecuted_ScriptJob(t *testing.T) {
	p := fixedPublisher()
	finished := time.Date(2026, 5, 19, 14, 0, 3, 0, time.UTC)
	result := model.JobResult{
		ExecutionID: "8f4b1c9e-0000-0000-0000-000000000001",
		ScheduleID:  "3a7d2b5f-0000-0000-0000-000000000002",
		JobID:       "9c1e8a4d-0000-0000-0000-000000000003",
		AgentID:     "agent-001",
		JobType:     model.JobTypeScript,
		Status:      model.JobStatusSuccess,
		StartedAt:   time.Date(2026, 5, 19, 14, 0, 1, 0, time.UTC),
		FinishedAt:  finished,
		Script: &model.ScriptResult{
			ExitCode: 0,
		},
	}
	target := model.Target{Type: model.TargetTypeScript, ID: "/opt/scripts/check_disk.sh"}
	e := p.buildJobExecuted(result, target)

	if e.Action != model.AuditActionJobExecuted {
		t.Errorf("Action = %q", e.Action)
	}
	if e.Target.Type != model.TargetTypeScript || e.Target.ID != "/opt/scripts/check_disk.sh" {
		t.Errorf("Target = %+v", e.Target)
	}
	if e.Result != model.EventResultSuccess {
		t.Errorf("Result = %q", e.Result)
	}
	if !e.OccurredAt.Equal(finished) {
		t.Errorf("OccurredAt = %v, want FinishedAt %v", e.OccurredAt, finished)
	}
	if e.Metadata["execution_id"] != result.ExecutionID {
		t.Errorf("metadata.execution_id = %v", e.Metadata["execution_id"])
	}
	if e.Metadata["job_type"] != string(model.JobTypeScript) {
		t.Errorf("metadata.job_type = %v", e.Metadata["job_type"])
	}
	if ec, _ := e.Metadata["exit_code"].(int); ec != 0 {
		t.Errorf("metadata.exit_code = %v (%T)", e.Metadata["exit_code"], e.Metadata["exit_code"])
	}
}

// TestBuildJobExecuted_LogJob_NoExitCode: LOG_JOB은 exit_code 없음.
// result.Status가 FAIL이면 EventResult도 FAIL로 매핑.
func TestBuildJobExecuted_LogJob_NoExitCode(t *testing.T) {
	p := fixedPublisher()
	finished := time.Date(2026, 5, 19, 14, 0, 2, 0, time.UTC)
	result := model.JobResult{
		ExecutionID: "x",
		ScheduleID:  "y",
		JobID:       "z",
		AgentID:     "agent-001",
		JobType:     model.JobTypeLog,
		Status:      model.JobStatusFail,
		FinishedAt:  finished,
		Log:         &model.LogResult{},
	}
	target := model.Target{Type: model.TargetTypeLogFile, ID: "/var/log/app/error.log"}
	e := p.buildJobExecuted(result, target)

	if e.Target.Type != model.TargetTypeLogFile {
		t.Errorf("Target.Type = %q", e.Target.Type)
	}
	if e.Result != model.EventResultFail {
		t.Errorf("Result = %q, want FAIL", e.Result)
	}
	if _, ok := e.Metadata["exit_code"]; ok {
		t.Errorf("LOG_JOB metadata should not include exit_code")
	}
}
