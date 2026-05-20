package model

import (
	"encoding/json"
	"reflect"
	"testing"
	"time"
)

// TestCommand_ScriptJob_Unmarshal: spec §5.1.1 예시와 동일 구조의 JSON을
// Command로 역직렬화 후, Spec을 ScriptJobSpec으로 다시 역직렬화하는
// 2단계 흐름을 검증한다.
func TestCommand_ScriptJob_Unmarshal(t *testing.T) {
	raw := `{
		"execution_id": "8f4b1c9e-0000-0000-0000-000000000001",
		"schedule_id": "3a7d2b5f-0000-0000-0000-000000000002",
		"job_id": "9c1e8a4d-0000-0000-0000-000000000003",
		"target_agent_id": "agent-001",
		"job_type": "SCRIPT_JOB",
		"issued_at": "2026-05-19T14:00:00Z",
		"valid_until": "2026-05-19T14:04:30Z",
		"spec": {
			"script_path": "/opt/scripts/check_disk.sh",
			"args": ["--threshold", "80"],
			"timeout_seconds": 30,
			"output_cap_bytes": 65536
		}
	}`

	var cmd Command
	if err := json.Unmarshal([]byte(raw), &cmd); err != nil {
		t.Fatalf("unmarshal command: %v", err)
	}

	if cmd.JobType != JobTypeScript {
		t.Errorf("JobType = %q, want %q", cmd.JobType, JobTypeScript)
	}
	if cmd.TargetAgentID != "agent-001" {
		t.Errorf("TargetAgentID = %q", cmd.TargetAgentID)
	}
	wantIssued := time.Date(2026, 5, 19, 14, 0, 0, 0, time.UTC)
	if !cmd.IssuedAt.Equal(wantIssued) {
		t.Errorf("IssuedAt = %v, want %v", cmd.IssuedAt, wantIssued)
	}
	wantValid := time.Date(2026, 5, 19, 14, 4, 30, 0, time.UTC)
	if !cmd.ValidUntil.Equal(wantValid) {
		t.Errorf("ValidUntil = %v, want %v", cmd.ValidUntil, wantValid)
	}

	var spec ScriptJobSpec
	if err := json.Unmarshal(cmd.Spec, &spec); err != nil {
		t.Fatalf("unmarshal script spec: %v", err)
	}
	want := ScriptJobSpec{
		ScriptPath:     "/opt/scripts/check_disk.sh",
		Args:           []string{"--threshold", "80"},
		TimeoutSeconds: 30,
		OutputCapBytes: 65536,
	}
	if !reflect.DeepEqual(spec, want) {
		t.Errorf("ScriptJobSpec mismatch:\n got=%+v\nwant=%+v", spec, want)
	}
}

// TestCommand_LogJob_Unmarshal: spec §5.1.2 예시와 동일 구조의 JSON
// 흐름을 검증한다.
func TestCommand_LogJob_Unmarshal(t *testing.T) {
	raw := `{
		"execution_id": "8f4b1c9e-0000-0000-0000-000000000001",
		"schedule_id": "3a7d2b5f-0000-0000-0000-000000000002",
		"job_id": "9c1e8a4d-0000-0000-0000-000000000003",
		"target_agent_id": "agent-001",
		"job_type": "LOG_JOB",
		"issued_at": "2026-05-19T14:00:00Z",
		"valid_until": "2026-05-19T14:04:30Z",
		"spec": {
			"log_path": "/var/log/app/error.log",
			"pattern": "ERROR|FATAL",
			"encoding": "UTF-8"
		}
	}`

	var cmd Command
	if err := json.Unmarshal([]byte(raw), &cmd); err != nil {
		t.Fatalf("unmarshal command: %v", err)
	}

	if cmd.JobType != JobTypeLog {
		t.Errorf("JobType = %q, want %q", cmd.JobType, JobTypeLog)
	}

	var spec LogJobSpec
	if err := json.Unmarshal(cmd.Spec, &spec); err != nil {
		t.Fatalf("unmarshal log spec: %v", err)
	}
	want := LogJobSpec{
		LogPath:  "/var/log/app/error.log",
		Pattern:  "ERROR|FATAL",
		Encoding: "UTF-8",
	}
	if !reflect.DeepEqual(spec, want) {
		t.Errorf("LogJobSpec mismatch:\n got=%+v\nwant=%+v", spec, want)
	}
}
