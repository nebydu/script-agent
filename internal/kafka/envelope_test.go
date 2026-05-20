package kafka

import (
	"testing"

	"monitoring/script-agent/internal/model"
)

// TestBuildHeaders_WithTraceID: trace_id가 있을 때 4개 헤더가 모두 채워진다.
func TestBuildHeaders_WithTraceID(t *testing.T) {
	h := BuildHeaders("msg-1", "trace-1")
	if len(h) != 4 {
		t.Fatalf("len = %d, want 4", len(h))
	}
	got := map[string]string{}
	for _, e := range h {
		got[e.Key] = string(e.Value)
	}
	if got[model.HeaderMessageID] != "msg-1" {
		t.Errorf("x-message-id = %q", got[model.HeaderMessageID])
	}
	if got[model.HeaderMessageVersion] != model.MessageVersion {
		t.Errorf("x-message-version = %q, want %q", got[model.HeaderMessageVersion], model.MessageVersion)
	}
	if got[model.HeaderSource] != model.SourceAgent {
		t.Errorf("x-source = %q, want %q", got[model.HeaderSource], model.SourceAgent)
	}
	if got[model.HeaderTraceID] != "trace-1" {
		t.Errorf("x-trace-id = %q", got[model.HeaderTraceID])
	}
}

// TestBuildHeaders_WithoutTraceID: trace_id가 빈 문자열이면 헤더에서
// 제외된다 (spec §2.2의 ○ 표기와 정합).
func TestBuildHeaders_WithoutTraceID(t *testing.T) {
	h := BuildHeaders("msg-2", "")
	if len(h) != 3 {
		t.Fatalf("len = %d, want 3", len(h))
	}
	for _, e := range h {
		if e.Key == model.HeaderTraceID {
			t.Errorf("x-trace-id should be omitted when empty")
		}
	}
}

// TestNewMessageID_UUIDForm: canonical UUID 문자열 길이는 36자.
func TestNewMessageID_UUIDForm(t *testing.T) {
	id := NewMessageID()
	if len(id) != 36 {
		t.Errorf("id len = %d (%q), want 36", len(id), id)
	}
}
