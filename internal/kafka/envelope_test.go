package kafka

import (
	"testing"

	kgo "github.com/segmentio/kafka-go"

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

// --- SourceFromHeaders 단위 테스트 (envelope §2.3 가드 고정) ---

// TestSourceFromHeaders_KnownValue: 알려진 값("script-agent") 헤더 →
// value="script-agent", present=true.
// envelope §2.3: x-source 알려진 값은 비규범 목록일 뿐, 이 함수는
// 단순히 값을 반환하며 검증 대조를 하지 않는다.
func TestSourceFromHeaders_KnownValue(t *testing.T) {
	headers := []kgo.Header{
		{Key: model.HeaderSource, Value: []byte("script-agent")},
	}
	value, present := SourceFromHeaders(headers)
	if !present {
		t.Fatal("present = false, want true")
	}
	if value != "script-agent" {
		t.Errorf("value = %q, want %q", value, "script-agent")
	}
}

// TestSourceFromHeaders_UnknownValue: 미지 값("unknown-future-service") 헤더 →
// value="unknown-future-service", present=true.
// 핵심 의도: 이 함수는 폐쇄 enum 검증을 하지 않으므로 미지 x-source가 들어와도
// skip/error를 반환하지 않고 값+존재여부만 돌려준다. dispatch를 막는 분기가
// 설계상 불가능함을 잠근다(envelope §2.3 가드 자산화).
func TestSourceFromHeaders_UnknownValue(t *testing.T) {
	headers := []kgo.Header{
		{Key: model.HeaderSource, Value: []byte("unknown-future-service")},
	}
	value, present := SourceFromHeaders(headers)
	if !present {
		t.Fatal("present = false, want true — 미지 x-source도 존재로 처리해야 한다")
	}
	if value != "unknown-future-service" {
		t.Errorf("value = %q, want %q", value, "unknown-future-service")
	}
}

// TestSourceFromHeaders_Absent: x-source 헤더 자체가 없으면 present=false.
// 부재여도 처리 흐름에 영향 없음을 고정.
func TestSourceFromHeaders_Absent(t *testing.T) {
	// x-source 헤더 없이 다른 헤더만 존재
	headers := []kgo.Header{
		{Key: model.HeaderMessageID, Value: []byte("some-id")},
		{Key: model.HeaderMessageVersion, Value: []byte(model.MessageVersion)},
	}
	value, present := SourceFromHeaders(headers)
	if present {
		t.Errorf("present = true, want false — 헤더 부재 시 false여야 한다")
	}
	if value != "" {
		t.Errorf("value = %q, want empty — 부재 시 빈 문자열을 반환해야 한다", value)
	}
}

// TestSourceFromHeaders_EmptyHeaders: 헤더 슬라이스 자체가 비어있을 때 →
// present=false. 패닉 없이 안전하게 처리됨을 확인.
func TestSourceFromHeaders_EmptyHeaders(t *testing.T) {
	value, present := SourceFromHeaders([]kgo.Header{})
	if present {
		t.Errorf("present = true, want false")
	}
	if value != "" {
		t.Errorf("value = %q, want empty", value)
	}
}

// TestSourceFromHeaders_EmptyValue: x-source 헤더가 있지만 값이 빈 문자열 →
// present=true, value="". 헤더 키 존재 자체를 감지함을 고정.
func TestSourceFromHeaders_EmptyValue(t *testing.T) {
	headers := []kgo.Header{
		{Key: model.HeaderSource, Value: []byte("")},
	}
	value, present := SourceFromHeaders(headers)
	if !present {
		t.Fatal("present = false, want true — 빈 값이라도 헤더 키가 있으면 present=true")
	}
	if value != "" {
		t.Errorf("value = %q, want empty", value)
	}
}

// TestSourceFromHeaders_NoSkipOnUnknown: 미지 x-source에 대해 이 함수가
// 오류·panic을 반환하지 않고 정상 값을 돌려주는 불변식.
// "값+존재여부만 반환" — dispatch 흐름을 깨는 side effect가 없음을 고정한다.
func TestSourceFromHeaders_NoSkipOnUnknown(t *testing.T) {
	unknownSources := []string{
		"unknown-future-service",
		"new-component-v2",
		"",
		"hub",
		"오픈텔레메트리",
	}
	for _, src := range unknownSources {
		headers := []kgo.Header{
			{Key: model.HeaderSource, Value: []byte(src)},
		}
		// 패닉이 없어야 하고, 반환된 값은 입력과 일치해야 한다.
		value, present := SourceFromHeaders(headers)
		if !present {
			t.Errorf("src=%q: present = false, want true", src)
		}
		if value != src {
			t.Errorf("src=%q: value = %q, want %q", src, value, src)
		}
	}
}
