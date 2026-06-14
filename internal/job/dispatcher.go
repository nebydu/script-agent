package job

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"monitoring/script-agent/internal/model"
)

// ResultPublisher는 Dispatcher가 JobResult를 보낼 곳이다.
// 실제 구현은 internal/jobresult.Publisher.
type ResultPublisher interface {
	Publish(ctx context.Context, result model.JobResult) error
}

// AuditPublisher는 Dispatcher가 JOB_EXECUTED 이벤트를 보낼 곳이다.
// 실제 구현은 internal/audit.Publisher.
type AuditPublisher interface {
	JobExecuted(ctx context.Context, result model.JobResult, target model.Target) error
}

// Dispatcher는 수신한 Command를 다음 규칙에 따라 동기로 처리한다:
//   - target_agent_id가 자기 것이 아니면 무시 (spec §5.1).
//   - valid_until이 지났으면 silent skip (spec §5.1).
//   - 그 외에는 Runner.Run을 호출하고, 결과를 job_type별 결과 토픽
//     (result-topic-job/result-topic-log)과 audit-topic 토픽에 각각
//     발행한 뒤 반환.
//
// 동기 처리의 이유: 호출 측(command-topic consumer)이 Dispatch 반환 후에만
// Kafka offset을 commit해야 at-least-once가 성립한다. 이전 비동기 구현은
// goroutine spawn 직후 commit이 일어나 at-most-once 손실이 가능했다 —
// Agent crash 시 진행 중이던 명령이 재처리되지 않음.
//
// 동일 schedule_id 재진입 방지: consumer loop가 단일 goroutine이고
// Dispatch가 동기이므로 동일 schedule이 동시에 두 번 실행되는 경로는
// 구조적으로 존재하지 않는다. 연속 발행된 명령은 valid_until 만료로
// 자연스럽게 skip된다.
//
// Cross-schedule 병렬성은 의도적으로 포기 — 운영 표준(Nagios/Zabbix 등)이
// agent worker 단위로 serial 처리한다. 본개발에서 worker pool이 필요해지면
// 그때 도입한다.
type Dispatcher struct {
	agentID string
	runners map[model.JobType]Runner
	results ResultPublisher
	auditor AuditPublisher
	logger  *slog.Logger
}

func NewDispatcher(
	agentID string,
	runners map[model.JobType]Runner,
	results ResultPublisher,
	auditor AuditPublisher,
	logger *slog.Logger,
) *Dispatcher {
	if logger == nil {
		logger = slog.Default()
	}
	return &Dispatcher{
		agentID: agentID,
		runners: runners,
		results: results,
		auditor: auditor,
		logger:  logger,
	}
}

// Dispatch는 한 Command를 동기로 처리하고 반환한다.
//
// 반환값 nil = "commit safe":
//   - target_agent_id 불일치 → 우리 명령 아님, commit으로 skip
//   - valid_until 만료 → spec §5.1 silent skip, commit
//   - 알 수 없는 job_type → spec 위반 drop, commit (재처리 무의미)
//   - Runner 실행 + 결과/감사 발행 모두 성공
//
// 반환값 non-nil = "commit 금지 / redeliver 필요":
//   - Runner 시스템 오류 (ctx 취소 등) → 결과 자체 의미 없음
//   - 결과 토픽 발행 실패 → audit 시도 없이 즉시 반환
//   - audit-topic 발행 실패
//
// 발행 순서가 results 먼저 → audit 나중인 이유: results 실패 시 audit을
// skip하면 "audit에는 JOB_EXECUTED 있는데 정작 결과는 없음"이라는 가장
// 혼란스러운 비대칭 실패를 막을 수 있다. 반대 케이스(results 성공 후
// audit 실패 → 재기동 시 results 중복)는 여전히 가능하지만 데이터는 보존
// 되므로 덜 나쁘다. 완전한 양방향 차단은 Kafka 트랜잭션 필요, 데모 범위
// 초과. BE는 execution_id로 dedup해야 한다.
//
// publish 실패도 error로 반환 — Kafka offset은 high-water mark라
// out-of-order commit을 허용하지 않는다. 호출 측은 publish 실패를 받으면
// commit하지 않고 consumer loop를 즉시 종료해야 last committed offset부터
// 정확하게 redeliver된다 (fail-fast at-least-once).
func (d *Dispatcher) Dispatch(ctx context.Context, cmd model.Command) error {
	if cmd.TargetAgentID != d.agentID {
		d.logger.Debug("command target mismatch — ignored",
			"target_agent_id", cmd.TargetAgentID,
			"execution_id", cmd.ExecutionID,
		)
		return nil
	}

	if time.Now().After(cmd.ValidUntil) {
		// spec §5.1: 지난 명령은 silent skip. 다음 정상 트리거 대기.
		d.logger.Debug("command expired — skipped",
			"execution_id", cmd.ExecutionID,
			"valid_until", cmd.ValidUntil.Format(time.RFC3339),
		)
		return nil
	}

	runner, ok := d.runners[cmd.JobType]
	if !ok {
		// model enum 외 값. spec 위반이지만 재시도해도 같은 결과 → drop +
		// commit (poison pill). 본개발에서 dead-letter 검토.
		d.logger.Error("unknown job_type — dropped",
			"job_type", cmd.JobType,
			"execution_id", cmd.ExecutionID,
		)
		return nil
	}

	result, target, err := runner.Run(ctx, d.agentID, cmd)
	if err != nil {
		// 시스템 레벨 실패(ctx 취소 등). 결과 의미 없으므로 발행 생략.
		d.logger.Warn("runner returned error — result not published",
			"error", err,
			"execution_id", cmd.ExecutionID,
			"job_type", cmd.JobType,
		)
		return fmt.Errorf("runner: %w", err)
	}

	// results 먼저. 실패 시 audit은 시도하지 않음 — godoc 참조.
	if err := d.results.Publish(ctx, result); err != nil {
		d.logger.Error("failed to publish job-result",
			"error", err,
			"execution_id", cmd.ExecutionID,
		)
		return fmt.Errorf("publish job-result: %w", err)
	}
	if err := d.auditor.JobExecuted(ctx, result, target); err != nil {
		d.logger.Error("failed to publish JOB_EXECUTED audit",
			"error", err,
			"execution_id", cmd.ExecutionID,
		)
		return fmt.Errorf("publish JOB_EXECUTED: %w", err)
	}
	return nil
}
