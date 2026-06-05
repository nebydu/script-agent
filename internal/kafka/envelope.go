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

// SourceFromHeaders는 envelope 헤더(spec §2.2)에서 x-source 값을 추출한다.
// BuildHeaders(쓰기)와 대칭인 읽기 헬퍼이며, model.HeaderSource 키를 찾아
// 값과 존재 여부만 반환하는 순수 함수다.
//
// 의도적으로 폐쇄 enum / allowlist 검증을 하지 않는다(envelope §2.3 —
// x-source의 알려진 값 목록은 비규범이며, consumer가 검증 대조할 대상이
// 아니다). 미지값이나 부재에도 명령 처리(dispatch)가 깨지지 않음을 의도된
// 가드로 명시하기 위해, 이 함수는 "값 추출 + 존재 여부"만 수행하고
// skip/error 판정 boolean을 만들지 않는다.
func SourceFromHeaders(headers []kgo.Header) (value string, present bool) {
	for _, h := range headers {
		if h.Key == model.HeaderSource {
			return string(h.Value), true
		}
	}
	return "", false
}
