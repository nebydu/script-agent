package model

import (
	"encoding/json"
	"testing"
	"time"
)

// TestJobResult_ScriptJob_Marshal: SCRIPT_JOB 결과는 spec §5.2.1 형식대로
// script 필드는 객체, log 필드는 null로 직렬화되어야 한다.
func TestJobResult_ScriptJob_Marshal(t *testing.T) {
	r := JobResult{
		ExecutionID: "8f4b1c9e-0000-0000-0000-000000000001",
		ScheduleID:  "3a7d2b5f-0000-0000-0000-000000000002",
		JobID:       "9c1e8a4d-0000-0000-0000-000000000003",
		AgentID:     "agent-001",
		JobType:     JobTypeScript,
		Status:      JobStatusSuccess,
		StartedAt:   time.Date(2026, 5, 19, 14, 0, 1, 0, time.UTC),
		FinishedAt:  time.Date(2026, 5, 19, 14, 0, 3, 0, time.UTC),
		Script: &ScriptResult{
			ExitCode:  0,
			StdoutCap: "Disk usage: 42%",
			StderrCap: "",
			Truncated: false,
		},
		Log: nil,
	}
	data, err := json.Marshal(r)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	// 필드 단위로 분해해 검증 — 키 순서 등 brittle하지 않게.
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(data, &raw); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	if s := string(raw["log"]); s != "null" {
		t.Errorf("log = %q, want null", s)
	}
	if s := string(raw["script"]); s == "null" {
		t.Errorf("script must be object, got null")
	}
	if s := string(raw["job_type"]); s != `"SCRIPT_JOB"` {
		t.Errorf("job_type = %s, want \"SCRIPT_JOB\"", s)
	}
	if s := string(raw["status"]); s != `"SUCCESS"` {
		t.Errorf("status = %s, want \"SUCCESS\"", s)
	}

	// script 내부도 검증.
	var script ScriptResult
	if err := json.Unmarshal(raw["script"], &script); err != nil {
		t.Fatalf("unmarshal script: %v", err)
	}
	if script.StdoutCap != "Disk usage: 42%" || script.ExitCode != 0 || script.Truncated {
		t.Errorf("ScriptResult roundtrip mismatch: %+v", script)
	}
}

// TestJobResult_LogJob_Marshal: LOG_JOB 결과는 spec §5.2.2 형식대로
// log 필드는 객체, script 필드는 null로 직렬화되어야 한다.
func TestJobResult_LogJob_Marshal(t *testing.T) {
	r := JobResult{
		ExecutionID: "8f4b1c9e-0000-0000-0000-000000000001",
		ScheduleID:  "3a7d2b5f-0000-0000-0000-000000000002",
		JobID:       "9c1e8a4d-0000-0000-0000-000000000003",
		AgentID:     "agent-001",
		JobType:     JobTypeLog,
		Status:      JobStatusSuccess,
		StartedAt:   time.Date(2026, 5, 19, 14, 0, 1, 0, time.UTC),
		FinishedAt:  time.Date(2026, 5, 19, 14, 0, 2, 0, time.UTC),
		Script:      nil,
		Log: &LogResult{
			MatchedLinesCount: 3,
			SampleLines: []string{
				"[2026-05-19 13:59:42] ERROR Failed to connect to DB",
				"[2026-05-19 13:59:55] ERROR Retry exceeded",
			},
		},
	}
	data, err := json.Marshal(r)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	var raw map[string]json.RawMessage
	if err := json.Unmarshal(data, &raw); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	if s := string(raw["script"]); s != "null" {
		t.Errorf("script = %q, want null", s)
	}
	if s := string(raw["log"]); s == "null" {
		t.Errorf("log must be object, got null")
	}
	if s := string(raw["job_type"]); s != `"LOG_JOB"` {
		t.Errorf("job_type = %s, want \"LOG_JOB\"", s)
	}

	var lr LogResult
	if err := json.Unmarshal(raw["log"], &lr); err != nil {
		t.Fatalf("unmarshal log: %v", err)
	}
	if lr.MatchedLinesCount != 3 || len(lr.SampleLines) != 2 {
		t.Errorf("LogResult roundtrip mismatch: %+v", lr)
	}
}
