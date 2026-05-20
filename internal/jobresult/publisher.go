// Package jobresult는 job-results 토픽으로 JobResult를 발행한다
// (spec §5.2). audit과 의미가 달라 별도 패키지로 둔다.
package jobresult

import (
	"context"
	"encoding/json"
	"fmt"

	"monitoring/script-agent/internal/kafka"
	"monitoring/script-agent/internal/model"
)

// Publisher는 job-results 토픽 발행자다.
type Publisher struct {
	writer *kafka.Writer
	topic  string
}

func NewPublisher(writer *kafka.Writer, topic string) *Publisher {
	return &Publisher{writer: writer, topic: topic}
}

// Publish는 JobResult를 JSON으로 직렬화해 발행한다. key는 result.AgentID
// (spec §2.3 — agent_id 기반 ordering).
func (p *Publisher) Publish(ctx context.Context, result model.JobResult) error {
	payload, err := json.Marshal(result)
	if err != nil {
		return fmt.Errorf("jobresult: marshal: %w", err)
	}
	headers := kafka.BuildHeaders(kafka.NewMessageID(), "")
	if err := p.writer.WriteMessage(ctx, p.topic, result.AgentID, payload, headers); err != nil {
		return fmt.Errorf("jobresult: write: %w", err)
	}
	return nil
}
