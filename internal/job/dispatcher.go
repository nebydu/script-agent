package job

import (
	"context"
	"log/slog"
	"sync"
	"sync/atomic"
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

// Dispatcher는 수신한 Command를 다음 규칙에 따라 dispatch한다:
//   - target_agent_id가 자기 것이 아니면 무시 (spec §5.1).
//   - valid_until이 지났으면 silent skip (spec §5.1).
//   - 동일 schedule_id가 이미 inflight면 skip — 재진입 방지
//     (Nagios/Zabbix 등 모니터링 표준 패턴, 사전 결정).
//   - 그 외에는 goroutine에서 Runner.Run을 실행하고, 결과를
//     job-results 토픽과 audit-events 토픽에 각각 발행.
//
// 호출 측(commands consumer loop)은 Dispatch를 직렬로 호출한다 — 그래야
// kafka offset이 순서대로 commit된다. Dispatch는 lock 획득 + goroutine
// spawn까지만 동기로 처리하고 즉시 반환하므로 consumer loop는 막히지 않음.
//
// 한계: inflight sync.Map은 schedule 수만큼 entry가 누적된다. 데모 (몇 개
// schedule)에서는 무의미하지만 본개발에서는 GC 정책 필요.
type Dispatcher struct {
	agentID  string
	runners  map[model.JobType]Runner
	results  ResultPublisher
	auditor  AuditPublisher
	logger   *slog.Logger
	inflight sync.Map // schedule_id(string) → *atomic.Bool

	wg sync.WaitGroup // 진행 중 Job 추적 (Wait로 graceful drain 가능)
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

// Dispatch는 한 Command를 처리한다. 비동기 실행이 필요한 경로(Runner.Run
// 호출)는 내부 goroutine에서 돌고, Dispatch 자체는 lock 결정까지만 하고
// 즉시 반환한다.
func (d *Dispatcher) Dispatch(ctx context.Context, cmd model.Command) {
	if cmd.TargetAgentID != d.agentID {
		d.logger.Debug("command target mismatch — ignored",
			"target_agent_id", cmd.TargetAgentID,
			"execution_id", cmd.ExecutionID,
		)
		return
	}

	if time.Now().After(cmd.ValidUntil) {
		// spec §5.1: 지난 명령은 silent skip. 다음 정상 트리거 대기.
		d.logger.Debug("command expired — skipped",
			"execution_id", cmd.ExecutionID,
			"valid_until", cmd.ValidUntil.Format(time.RFC3339),
		)
		return
	}

	// per-schedule 재진입 방지.
	v, _ := d.inflight.LoadOrStore(cmd.ScheduleID, &atomic.Bool{})
	flag := v.(*atomic.Bool)
	if !flag.CompareAndSwap(false, true) {
		d.logger.Debug("schedule re-entry — skipped",
			"schedule_id", cmd.ScheduleID,
			"execution_id", cmd.ExecutionID,
		)
		return
	}

	runner, ok := d.runners[cmd.JobType]
	if !ok {
		// model enum 외 값. spec 위반. 락 풀고 종료.
		flag.Store(false)
		d.logger.Error("unknown job_type — dropped",
			"job_type", cmd.JobType,
			"execution_id", cmd.ExecutionID,
		)
		return
	}

	d.wg.Add(1)
	go func() {
		defer d.wg.Done()
		defer flag.Store(false)

		result, target, err := runner.Run(ctx, d.agentID, cmd)
		if err != nil {
			// 시스템 레벨 실패(ctx 취소 등). 발행 생략.
			d.logger.Warn("runner returned error — result not published",
				"error", err,
				"execution_id", cmd.ExecutionID,
				"job_type", cmd.JobType,
			)
			return
		}

		if err := d.results.Publish(ctx, result); err != nil {
			d.logger.Warn("failed to publish job-result",
				"error", err,
				"execution_id", cmd.ExecutionID,
			)
		}
		if err := d.auditor.JobExecuted(ctx, result, target); err != nil {
			d.logger.Warn("failed to publish JOB_EXECUTED audit",
				"error", err,
				"execution_id", cmd.ExecutionID,
			)
		}
	}()
}

// Wait는 진행 중인 모든 Job goroutine이 종료될 때까지 대기한다.
// graceful shutdown 경로에서 호출 (5s 타임아웃 안에서).
func (d *Dispatcher) Wait() {
	d.wg.Wait()
}
