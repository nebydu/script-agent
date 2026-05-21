package job

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"monitoring/script-agent/internal/model"
)

// fakeRunner는 호출 받으면 release 채널이 닫힐 때까지 대기한다.
// 동기 동작 검증(반환 차단 여부)에 사용.
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
	err      error // 설정 시 Publish가 항상 err 반환
}

func (p *fakeResults) Publish(ctx context.Context, result model.JobResult) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.err != nil {
		return p.err
	}
	p.received = append(p.received, result)
	return nil
}

type fakeAudit struct {
	mu       sync.Mutex
	received int
	err      error // 설정 시 JobExecuted가 항상 err 반환
}

func (p *fakeAudit) JobExecuted(ctx context.Context, result model.JobResult, target model.Target) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.err != nil {
		return p.err
	}
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
// 호출하지 않고 즉시 반환 (commit safe = nil).
func TestDispatcher_SkipsOnTargetMismatch(t *testing.T) {
	runner := newFakeRunner()
	close(runner.release) // 호출돼도 즉시 진행되도록 (호출 자체가 일어나면 안 됨)
	results := &fakeResults{}
	auditor := &fakeAudit{}
	d := NewDispatcher("agent-self", map[model.JobType]Runner{
		model.JobTypeScript: runner,
	}, results, auditor, nil)

	if err := d.Dispatch(context.Background(), newCmd("e1", "s1", "agent-other")); err != nil {
		t.Errorf("Dispatch err = %v, want nil (target mismatch는 commit safe)", err)
	}

	if got := runner.calls.Load(); got != 0 {
		t.Errorf("runner.calls = %d, want 0", got)
	}
	if len(results.received) != 0 {
		t.Errorf("results.received = %d, want 0", len(results.received))
	}
}

// TestDispatcher_SkipsOnExpired: valid_until이 지난 명령은 silent skip + nil.
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
	if err := d.Dispatch(context.Background(), cmd); err != nil {
		t.Errorf("Dispatch err = %v, want nil (expired는 commit safe)", err)
	}

	if got := runner.calls.Load(); got != 0 {
		t.Errorf("runner.calls = %d, want 0", got)
	}
}

// TestDispatcher_PublishesAfterRunBeforeReturn: 정상 명령 처리 시 Dispatch
// 반환 시점에 결과/감사 발행이 모두 완료되어 있어야 한다 (commit-after-
// processing의 핵심 보장). 반환값은 nil.
func TestDispatcher_PublishesAfterRunBeforeReturn(t *testing.T) {
	runner := newFakeRunner()
	close(runner.release)
	results := &fakeResults{}
	auditor := &fakeAudit{}
	d := NewDispatcher("agent-self", map[model.JobType]Runner{
		model.JobTypeScript: runner,
	}, results, auditor, nil)

	if err := d.Dispatch(context.Background(), newCmd("e1", "s1", "agent-self")); err != nil {
		t.Errorf("Dispatch err = %v, want nil", err)
	}

	if got := runner.calls.Load(); got != 1 {
		t.Errorf("runner.calls = %d, want 1", got)
	}
	if len(results.received) != 1 {
		t.Errorf("results.received = %d, want 1", len(results.received))
	}
	if auditor.received != 1 {
		t.Errorf("auditor.received = %d, want 1", auditor.received)
	}
}

// TestDispatcher_ResultsPublishFailureReturnsError: job-results 발행 실패는
// error 반환 + audit은 시도하지 않음 (비대칭 실패 시 "audit만 남는" 케이스
// 차단 — godoc 참조).
func TestDispatcher_ResultsPublishFailureReturnsError(t *testing.T) {
	runner := newFakeRunner()
	close(runner.release)
	publishErr := errors.New("broker unreachable")
	results := &fakeResults{err: publishErr}
	auditor := &fakeAudit{}
	d := NewDispatcher("agent-self", map[model.JobType]Runner{
		model.JobTypeScript: runner,
	}, results, auditor, nil)

	err := d.Dispatch(context.Background(), newCmd("e1", "s1", "agent-self"))
	if err == nil {
		t.Fatalf("Dispatch err = nil, want non-nil (publish 실패는 commit 금지 신호)")
	}
	if !errors.Is(err, publishErr) {
		t.Errorf("Dispatch err = %v, want wrap of %v", err, publishErr)
	}
	if auditor.received != 0 {
		t.Errorf("auditor.received = %d, want 0 (results 실패 시 audit skip)", auditor.received)
	}
}

// TestDispatcher_AuditPublishFailureReturnsError: audit-events 발행 실패도
// error 반환 (job-results 성공 여부 무관).
func TestDispatcher_AuditPublishFailureReturnsError(t *testing.T) {
	runner := newFakeRunner()
	close(runner.release)
	publishErr := errors.New("audit broker down")
	results := &fakeResults{}
	auditor := &fakeAudit{err: publishErr}
	d := NewDispatcher("agent-self", map[model.JobType]Runner{
		model.JobTypeScript: runner,
	}, results, auditor, nil)

	err := d.Dispatch(context.Background(), newCmd("e1", "s1", "agent-self"))
	if err == nil {
		t.Fatalf("Dispatch err = nil, want non-nil")
	}
	if !errors.Is(err, publishErr) {
		t.Errorf("Dispatch err = %v, want wrap of %v", err, publishErr)
	}
	// job-results는 성공했어도 audit 실패면 전체가 fail-fast.
	if len(results.received) != 1 {
		t.Errorf("results.received = %d, want 1 (job-results는 publish됐어야)", len(results.received))
	}
}

// TestDispatcher_BlocksUntilRunnerCompletes: Dispatch는 Runner가 끝날 때까지
// 블로킹된다. 호출 측의 offset commit이 처리 완료 전에 일어나지 않도록
// 하는 핵심 보장.
func TestDispatcher_BlocksUntilRunnerCompletes(t *testing.T) {
	runner := newFakeRunner()
	results := &fakeResults{}
	auditor := &fakeAudit{}
	d := NewDispatcher("agent-self", map[model.JobType]Runner{
		model.JobTypeScript: runner,
	}, results, auditor, nil)

	returned := make(chan struct{})
	go func() {
		_ = d.Dispatch(context.Background(), newCmd("e1", "s1", "agent-self"))
		close(returned)
	}()

	// runner가 release 대기 중이므로 Dispatch는 반환하면 안 된다.
	select {
	case <-returned:
		t.Fatalf("Dispatch returned before runner released — sync 보장 깨짐")
	case <-time.After(50 * time.Millisecond):
		// expected — 아직 블로킹 중
	}

	close(runner.release)

	select {
	case <-returned:
		// good — runner 완료 후 정상 반환
	case <-time.After(time.Second):
		t.Fatalf("Dispatch did not return within 1s after runner release")
	}

	if len(results.received) != 1 {
		t.Errorf("results.received = %d, want 1", len(results.received))
	}
}

// TestDispatcher_CtxCancelReturnsError: Runner가 ctx 취소로 error를 반환하면
// 결과 발행은 생략되고 Dispatch도 error 반환 (commit 금지 신호).
func TestDispatcher_CtxCancelReturnsError(t *testing.T) {
	runner := newFakeRunner() // release 안 함 — ctx 취소로만 종료 유도
	results := &fakeResults{}
	auditor := &fakeAudit{}
	d := NewDispatcher("agent-self", map[model.JobType]Runner{
		model.JobTypeScript: runner,
	}, results, auditor, nil)

	ctx, cancel := context.WithCancel(context.Background())
	type result struct{ err error }
	done := make(chan result, 1)
	go func() {
		done <- result{err: d.Dispatch(ctx, newCmd("e1", "s1", "agent-self"))}
	}()

	// runner가 진입할 시간을 주고 ctx cancel.
	time.Sleep(20 * time.Millisecond)
	cancel()

	var got result
	select {
	case got = <-done:
	case <-time.After(time.Second):
		t.Fatalf("Dispatch did not return after ctx cancel")
	}

	if got.err == nil {
		t.Errorf("Dispatch err = nil, want non-nil (runner ctx cancel)")
	}
	if len(results.received) != 0 {
		t.Errorf("results.received = %d, want 0 (cancel 경로는 발행 생략)", len(results.received))
	}
	if auditor.received != 0 {
		t.Errorf("auditor.received = %d, want 0", auditor.received)
	}
}
