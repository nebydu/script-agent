// Package kafka는 kafka-go 라이브러리를 Agent 용도로 얇게 래핑한다.
// envelope 헤더(spec §2.2) 생성, Writer/Reader 구성을 한 곳에 모아
// 호출 측(main, audit, jobresult)이 spec 세부를 다시 다루지 않게 한다.
package kafka

import (
	"github.com/google/uuid"
	kgo "github.com/segmentio/kafka-go"

	"monitoring/script-agent/internal/model"
)

// BuildHeaders는 spec §2.2의 envelope 헤더를 채워 kafka.Header 슬라이스로
// 반환한다. messageID는 호출 측이 생성해 넘겨준다 — 같은 메시지를
// 재발행할 때 동일 ID를 유지하기 위함(데모는 재발행 미사용이지만
// 인터페이스를 그렇게 둠).
//
// traceID는 옵셔널이다 (spec §2.2의 ○). 빈 문자열이면 헤더에서 생략한다.
func BuildHeaders(messageID, traceID string) []kgo.Header {
	hdrs := []kgo.Header{
		{Key: model.HeaderMessageID, Value: []byte(messageID)},
		{Key: model.HeaderMessageVersion, Value: []byte(model.MessageVersion)},
		{Key: model.HeaderSource, Value: []byte(model.SourceAgent)},
	}
	if traceID != "" {
		hdrs = append(hdrs, kgo.Header{Key: model.HeaderTraceID, Value: []byte(traceID)})
	}
	return hdrs
}

// NewMessageID는 새 x-message-id 값을 생성한다. UUIDv4.
func NewMessageID() string {
	return uuid.NewString()
}
