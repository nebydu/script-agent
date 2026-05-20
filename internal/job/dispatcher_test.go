package job

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"monitoring/script-agent/internal/model"
)

// fakeRunner는 호출 받으면 release 채널이 닫힐 때까지 대기한다.
// 동일 호출 횟수를 calls로 카운트.
type fakeRunner struct {
	release chan struct{}
	calls   atomic.Int32
}

func newFakeRunner() *fakeRunner {
	return &fakeRunner{release: make(chan struct{})}
}

func (r *fakeRunner) Run(ctx context.Context, agentID string, cmd model.Command) (model.JobResult, model.Target, error) {
	r.calls.Add(1)
	select {
	case <-r.release:
	case <-ctx.Done():
		return model.JobResult{}, model.Target{}, ctx.Err()
	}
	return model.JobResult{
		ExecutionID: cmd.ExecutionID,
		ScheduleID:  cmd.ScheduleID,
		JobID:       cmd.JobID,
		AgentID:     agentID,
		JobType:     cmd.JobType,
		Status:      model.JobStatusSuccess,
		StartedAt:   time.Now().UTC(),
		FinishedAt:  time.Now().UTC(),
		Script:      &model.ScriptResult{ExitCode: 0},
	}, model.Target{Type: model.TargetTypeScript, ID: "/x"}, nil
}

type fakeResults struct {
	mu       sync.Mutex
	received []model.JobResult
}

func (p *fakeResults) Publish(ctx context.Context, result model.JobResult) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.received = append(p.received, result)
	return nil
}

type fakeAudit struct {
	mu       sync.Mutex
	received int
}

func (p *fakeAudit) JobExecuted(ctx context.Context, result model.JobResult, target model.Target) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.received++
	return nil
}

func newCmd(execID, scheduleID, agentID string) model.Command {
	return model.Command{
		ExecutionID:   execID,
		ScheduleID:    scheduleID,
		JobID:         "job-" + scheduleID,
		TargetAgentID: agentID,
		JobType:       model.JobTypeScript,
		IssuedAt:      time.Now().UTC(),
		ValidUntil:    time.Now().UTC().Add(time.Hour),
	}
}

// TestDispatcher_SkipsOnTargetMismatch: 다른 Agent로 향한 명령은 Runner를
// 호출하지 않는다.
func TestDispatcher_SkipsOnTargetMismatch(t *testing.T) {
	runner := newFakeRunner()
	close(runner.release) // 즉시 진행
	results := &fakeResults{}
	auditor := &fakeAudit{}
	d := NewDispatcher("agent-self", map[model.JobType]Runner{
		model.JobTypeScript: runner,
	}, results, auditor, nil)

	d.Dispatch(context.Background(), newCmd("e1", "s1", "agent-other"))
	d.Wait()

	if got := runner.calls.Load(); got != 0 {
		t.Errorf("runner.calls = %d, want 0", got)
	}
	if len(results.received) != 0 {
		t.Errorf("results.received = %d, want 0", len(results.received))
	}
}

// TestDispatcher_SkipsOnExpired: valid_until이 지난 명령은 silent skip.
func TestDispatcher_SkipsOnExpired(t *testing.T) {
	runner := newFakeRunner()
	close(runner.release)
	results := &fakeResults{}
	auditor := &fakeAudit{}
	d := NewDispatcher("agent-self", map[model.JobType]Runner{
		model.JobTypeScript: runner,
	}, results, auditor, nil)

	cmd := newCmd("e1", "s1", "agent-self")
	cmd.ValidUntil = time.Now().UTC().Add(-time.Second) // 이미 만료
	d.Dispatch(context.Background(), cmd)
	d.Wait()

	if got := runner.calls.Load(); got != 0 {
		t.Errorf("runner.calls = %d, want 0", got)
	}
}

// TestDispatcher_SerialReentryGetsSkipped: 동일 schedule_id가 inflight
// 상태일 때 두 번째 호출은 즉시 skip.
func TestDispatcher_SerialReentryGetsSkipped(t *testing.T) {
	runner := newFakeRunner()
	results := &fakeResults{}
	auditor := &fakeAudit{}
	d := NewDispatcher("agent-self", map[model.JobType]Runner{
		model.JobTypeScript: runner,
	}, results, auditor, nil)

	d.Dispatch(context.Background(), newCmd("e1", "sched-A", "agent-self"))
	// 첫 호출이 goroutine에서 release 대기. flag.Set 보장 위해 잠시 대기.
	time.Sleep(20 * time.Millisecond)

	d.Dispatch(context.Background(), newCmd("e2", "sched-A", "agent-self"))
	// 두 번째는 lock 못 얻고 즉시 반환.

	if got := runner.calls.Load(); got != 1 {
		t.Errorf("runner.calls before release = %d, want 1", got)
	}

	close(runner.release)
	d.Wait()

	if got := runner.calls.Load(); got != 1 {
		t.Errorf("runner.calls after release = %d, want 1 (second was skipped)", got)
	}
	if len(results.received) != 1 {
		t.Errorf("results.received = %d, want 1", len(results.received))
	}
}

// TestDispatcher_DifferentSchedulesRunInParallel: 서로 다른 schedule_id는
// 동시에 진행된다.
func TestDispatcher_DifferentSchedulesRunInParallel(t *testing.T) {
	runner := newFakeRunner()
	results := &fakeResults{}
	auditor := &fakeAudit{}
	d := NewDispatcher("agent-self", map[model.JobType]Runner{
		model.JobTypeScript: runner,
	}, results, auditor, nil)

	d.Dispatch(context.Background(), newCmd("e1", "sched-A", "agent-self"))
	d.Dispatch(context.Background(), newCmd("e2", "sched-B", "agent-self"))

	// 두 goroutine 모두 release 대기 중이어야 함.
	// 잠시 대기 후 calls 카운트 검증.
	deadline := time.Now().Add(500 * time.Millisecond)
	for time.Now().Before(deadline) {
		if runner.calls.Load() == 2 {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	if got := runner.calls.Load(); got != 2 {
		t.Fatalf("runner.calls = %d, want 2 (parallel)", got)
	}

	close(runner.release)
	d.Wait()

	if len(results.received) != 2 {
		t.Errorf("results.received = %d, want 2", len(results.received))
	}
}
