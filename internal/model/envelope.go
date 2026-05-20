package model

// Kafka envelope 헤더 키 (spec §2.2).
//
// 도메인 데이터가 아닌 메타데이터(message_id / version / source /
// trace_id)는 Kafka 헤더로 분리하고, payload에는 도메인 데이터만 둔다.
const (
	HeaderMessageID      = "x-message-id"
	HeaderMessageVersion = "x-message-version"
	HeaderSource         = "x-source"
	HeaderTraceID        = "x-trace-id"
)

// envelope 고정값 (spec §2.2).
const (
	// MessageVersion은 payload 스키마 버전. 데모는 "1" 고정.
	MessageVersion = "1"

	// SourceAgent는 x-source 헤더값. Script Agent가 발행자임을 표시.
	SourceAgent = "script-agent"
)
