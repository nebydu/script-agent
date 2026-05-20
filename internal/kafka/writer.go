package kafka

import (
	"context"
	"strings"

	kgo "github.com/segmentio/kafka-go"
)

// Writer는 kafka-go Writer를 Agent용으로 래핑한다.
//
// 기본 정책:
//   - RequiredAcks: RequireAll (reference implementation 규율).
//     데모는 replication=1이라 비용 동일.
//   - Balancer: Hash. 메시지 키(agent_id 또는 target_agent_id)를 기준으로
//     파티션이 결정되어 Agent 단위 ordering이 보장된다 (spec §2.3).
//   - Async=false (기본). WriteMessages가 동기 대기하므로 호출 측의
//     ctx 만료가 그대로 발행 timeout이 된다.
//
// 단일 Writer가 여러 토픽을 처리한다. 메시지마다 Topic을 지정.
type Writer struct {
	w *kgo.Writer
}

// NewWriter는 콤마로 구분된 brokers 문자열을 받아 Writer를 생성한다.
// 빈 문자열은 허용하지 않는다 (호출 측이 config 검증).
func NewWriter(brokers string) *Writer {
	w := &kgo.Writer{
		Addr:         kgo.TCP(splitBrokers(brokers)...),
		Balancer:     &kgo.Hash{},
		RequiredAcks: kgo.RequireAll,
	}
	return &Writer{w: w}
}

// WriteMessage는 단일 메시지를 발행한다. RequireAll + Async=false 조합으로
// 반환 시점에 broker ack 완료가 보장된다 (ctx 만료 또는 broker 오류 외).
func (w *Writer) WriteMessage(ctx context.Context, topic, key string, payload []byte, headers []kgo.Header) error {
	return w.w.WriteMessages(ctx, kgo.Message{
		Topic:   topic,
		Key:     []byte(key),
		Value:   payload,
		Headers: headers,
	})
}

// Close는 Writer를 닫는다. kafka-go는 Close 안에서 pending 메시지를
// 정리한다. Async=false이므로 보통 pending이 없어 빠르게 반환.
func (w *Writer) Close() error {
	return w.w.Close()
}

func splitBrokers(s string) []string {
	parts := strings.Split(s, ",")
	out := parts[:0]
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p != "" {
			out = append(out, p)
		}
	}
	return out
}
