package job

import (
	"context"
	"encoding/json"
	"errors"
	"os/exec"
	"runtime"
	"strings"
	"testing"
	"time"

	"monitoring/script-agent/internal/model"
)

// TestClassifyScriptRun_Success: nil error + ctx 정상 → SUCCESS, exit 0.
func TestClassifyScriptRun_Success(t *testing.T) {
	status, exit := classifyScriptRun(context.Background(), nil, nil)
	if status != model.JobStatusSuccess {
		t.Errorf("status = %q", status)
	}
	if exit != 0 {
		t.Errorf("exit = %d", exit)
	}
}

// TestClassifyScriptRun_TimeoutOverridesExitErr: ctx deadline exceeded면
// exec.ExitError보다 우선 TIMEOUT.
func TestClassifyScriptRun_TimeoutOverridesExitErr(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 0)
	defer cancel()
	time.Sleep(time.Millisecond) // ctx 만료 보장
	// 가짜 ExitError (실제 exec 안 함)
	fakeErr := &exec.ExitError{}
	status, _ := classifyScriptRun(ctx, fakeErr, nil)
	if status != model.JobStatusTimeout {
		t.Errorf("status = %q, want TIMEOUT", status)
	}
}

// TestClassifyScriptRun_ExitErrFail: ctx 정상 + exec.ExitError → FAIL,
// exit code 보존.
func TestClassifyScriptRun_ExitErrFail(t *testing.T) {
	// exit code 추출에는 ProcessState가 필요. 진짜 실행해서 받자.
	var process *exec.Cmd
	if runtime.GOOS == "windows" {
		process = exec.Command("cmd", "/c", "exit /b 7")
	} else {
		process = exec.Command("sh", "-c", "exit 7")
	}
	err := process.Run()
	if err == nil {
		t.Fatalf("expected exit error")
	}
	status, exit := classifyScriptRun(context.Background(), err, process.ProcessState)
	if status != model.JobStatusFail {
		t.Errorf("status = %q", status)
	}
	if exit != 7 {
		t.Errorf("exit = %d, want 7", exit)
	}
}

// TestClassifyScriptRun_ExecMissingFail: 일반 에러(존재하지 않는 명령) →
// FAIL, exit -1.
func TestClassifyScriptRun_ExecMissingFail(t *testing.T) {
	err := errors.New("exec: \"nope\": not found")
	status, exit := classifyScriptRun(context.Background(), err, nil)
	if status != model.JobStatusFail {
		t.Errorf("status = %q", status)
	}
	if exit != -1 {
		t.Errorf("exit = %d, want -1", exit)
	}
}

// TestScriptRunner_Run_Echo: 실제 exec 통한 round-trip. SUCCESS, stdout
// 비어있지 않음. cross-platform shell 분기.
func TestScriptRunner_Run_Echo(t *testing.T) {
	spec := model.ScriptJobSpec{
		TimeoutSeconds: 5,
		OutputCapBytes: 1024,
	}
	if runtime.GOOS == "windows" {
		spec.ScriptPath = "cmd"
		spec.Args = []string{"/c", "echo", "hello"}
	} else {
		spec.ScriptPath = "sh"
		spec.Args = []string{"-c", "echo hello"}
	}
	specJSON, err := json.Marshal(spec)
	if err != nil {
		t.Fatalf("marshal spec: %v", err)
	}
	cmd := model.Command{
		ExecutionID:   "e1",
		ScheduleID:    "s1",
		JobID:         "j1",
		TargetAgentID: "agent-1",
		JobType:       model.JobTypeScript,
		Spec:          specJSON,
	}

	r := NewScriptRunner()
	result, target, runErr := r.Run(context.Background(), "agent-1", cmd)
	if runErr != nil {
		t.Fatalf("Run returned error: %v", runErr)
	}

	if result.Status != model.JobStatusSuccess {
		t.Errorf("Status = %q, want SUCCESS (stderr: %q)", result.Status, result.Script.StderrCap)
	}
	if result.Script.ExitCode != 0 {
		t.Errorf("ExitCode = %d, want 0", result.Script.ExitCode)
	}
	if !strings.Contains(result.Script.StdoutCap, "hello") {
		t.Errorf("StdoutCap = %q, want to contain 'hello'", result.Script.StdoutCap)
	}
	if result.Script.Truncated {
		t.Errorf("Truncated should be false")
	}
	if target.Type != model.TargetTypeScript {
		t.Errorf("target.Type = %q", target.Type)
	}
	if !result.StartedAt.Before(result.FinishedAt) && !result.StartedAt.Equal(result.FinishedAt) {
		t.Errorf("StartedAt %v should be <= FinishedAt %v", result.StartedAt, result.FinishedAt)
	}
}

// TestScriptRunner_Run_InvalidSpec: 깨진 spec JSON은 FAIL로 변환.
func TestScriptRunner_Run_InvalidSpec(t *testing.T) {
	cmd := model.Command{
		ExecutionID: "e1",
		JobType:     model.JobTypeScript,
		Spec:        json.RawMessage(`{"timeout_seconds": "not-a-number"}`),
	}
	r := NewScriptRunner()
	result, _, runErr := r.Run(context.Background(), "agent-1", cmd)
	if runErr != nil {
		t.Fatalf("Run returned error: %v", runErr)
	}
	if result.Status != model.JobStatusFail {
		t.Errorf("Status = %q, want FAIL", result.Status)
	}
}

