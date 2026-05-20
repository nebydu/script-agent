package model

import (
	"encoding/json"
	"testing"
	"time"
)

// TestAuditEvent_AgentStarted_Marshal: spec §5.3.1 예시 형식대로
// 직렬화되고 round-trip이 보존되는지 검증.
func TestAuditEvent_AgentStarted_Marshal(t *testing.T) {
	e := AuditEvent{
		EventID:    "11111111-1111-1111-1111-111111111111",
		Actor:      Actor{Type: ActorTypeAgent, ID: "agent-001"},
		Action:     AuditActionAgentStarted,
		Target:     Target{Type: TargetTypeAgent, ID: "agent-001"},
		Result:     EventResultSuccess,
		OccurredAt: time.Date(2026, 5, 19, 13, 55, 0, 0, time.UTC),
		Metadata: map[string]any{
			"hostname":      "demo-host-01",
			"os":            "linux/amd64",
			"agent_version": "0.1.0",
			"started_at":    "2026-05-19T13:55:00Z",
		},
	}
	data, err := json.Marshal(e)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	var got AuditEvent
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	if got.Action != AuditActionAgentStarted {
		t.Errorf("Action = %q", got.Action)
	}
	if got.Actor.Type != ActorTypeAgent || got.Actor.ID != "agent-001" {
		t.Errorf("Actor = %+v", got.Actor)
	}
	if got.Target.Type != TargetTypeAgent || got.Target.ID != "agent-001" {
		t.Errorf("Target = %+v", got.Target)
	}
	if got.Result != EventResultSuccess {
		t.Errorf("Result = %q", got.Result)
	}
	if hn, _ := got.Metadata["hostname"].(string); hn != "demo-host-01" {
		t.Errorf("metadata.hostname = %v", got.Metadata["hostname"])
	}
}

// TestAuditEvent_AgentStopped_Marshal: spec §5.3.2 — metadata.reason
// 보존 확인.
func TestAuditEvent_AgentStopped_Marshal(t *testing.T) {
	e := AuditEvent{
		EventID:    "22222222-2222-2222-2222-222222222222",
		Actor:      Actor{Type: ActorTypeAgent, ID: "agent-001"},
		Action:     AuditActionAgentStopped,
		Target:     Target{Type: TargetTypeAgent, ID: "agent-001"},
		Result:     EventResultSuccess,
		OccurredAt: time.Date(2026, 5, 19, 18, 30, 0, 0, time.UTC),
		Metadata:   map[string]any{"reason": "SIGTERM"},
	}
	data, err := json.Marshal(e)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	var got AuditEvent
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if got.Action != AuditActionAgentStopped {
		t.Errorf("Action = %q", got.Action)
	}
	if reason, _ := got.Metadata["reason"].(string); reason != "SIGTERM" {
		t.Errorf("metadata.reason = %v", got.Metadata["reason"])
	}
}

// TestAuditEvent_JobExecuted_Marshal: spec §5.3.3 — target.type이
// SCRIPT인 경우와 metadata 상관 키(execution_id 등)가 round-trip
// 보존되는지 확인.
func TestAuditEvent_JobExecuted_Marshal(t *testing.T) {
	e := AuditEvent{
		EventID:    "33333333-3333-3333-3333-333333333333",
		Actor:      Actor{Type: ActorTypeAgent, ID: "agent-001"},
		Action:     AuditActionJobExecuted,
		Target:     Target{Type: TargetTypeScript, ID: "/opt/scripts/check_disk.sh"},
		Result:     EventResultSuccess,
		OccurredAt: time.Date(2026, 5, 19, 14, 0, 3, 0, time.UTC),
		Metadata: map[string]any{
			"execution_id": "8f4b1c9e-0000-0000-0000-000000000001",
			"schedule_id":  "3a7d2b5f-0000-0000-0000-000000000002",
			"job_id":       "9c1e8a4d-0000-0000-0000-000000000003",
			"job_type":     "SCRIPT_JOB",
			"exit_code":    0,
		},
	}
	data, err := json.Marshal(e)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	var got AuditEvent
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	if got.Action != AuditActionJobExecuted {
		t.Errorf("Action = %q", got.Action)
	}
	if got.Target.Type != TargetTypeScript {
		t.Errorf("Target.Type = %q, want SCRIPT", got.Target.Type)
	}
	if got.Target.ID != "/opt/scripts/check_disk.sh" {
		t.Errorf("Target.ID = %q", got.Target.ID)
	}
	if eid, _ := got.Metadata["execution_id"].(string); eid != "8f4b1c9e-0000-0000-0000-000000000001" {
		t.Errorf("metadata.execution_id = %v", got.Metadata["execution_id"])
	}
	// JSON number → map[string]any 역직렬화 시 float64로 들어감 (Go 표준 동작).
	if ec, ok := got.Metadata["exit_code"].(float64); !ok || ec != 0 {
		t.Errorf("metadata.exit_code = %v (%T)", got.Metadata["exit_code"], got.Metadata["exit_code"])
	}
}
