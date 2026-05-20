package kafka

import (
	kgo "github.com/segmentio/kafka-go"
)

// Reader는 kafka-go consumer group Reader를 Agent용으로 래핑한다.
//
// 기본 정책 (spec ADR #4, 사전 확정):
//   - GroupID: "script-agent-<agent_id>" — Agent별 unique consumer group.
//     모든 Agent가 동일 commands 토픽에서 전체 메시지를 받고
//     payload.target_agent_id로 자기 것을 필터링한다 (spec §5.1).
//   - StartOffset: LastOffset (latest). 새 consumer group 생성 시점
//     이후의 명령만 구독. 모니터링 Agent 표준.
//
// auto-commit은 사용하지 않는다 — FetchMessage + CommitMessages 패턴으로
// 처리 완료 후에만 offset을 advance한다.
type Reader struct {
	r *kgo.Reader
}

// NewReader는 commands 토픽용 Reader를 만든다.
func NewReader(brokers, topic, agentID string) *Reader {
	r := kgo.NewReader(kgo.ReaderConfig{
		Brokers:     splitBrokers(brokers),
		Topic:       topic,
		GroupID:     "script-agent-" + agentID,
		StartOffset: kgo.LastOffset,
	})
	return &Reader{r: r}
}

// Underlying는 호출 측이 kafka-go 타입을 직접 다루어야 할 때 사용한다.
// (main 의 consume 루프가 FetchMessage / CommitMessages를 직접 호출.)
func (r *Reader) Underlying() *kgo.Reader {
	return r.r
}

// Close는 Reader를 닫는다.
func (r *Reader) Close() error {
	return r.r.Close()
}
