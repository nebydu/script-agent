package job

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"time"

	"monitoring/script-agent/internal/model"
)

// ScriptRunner는 SCRIPT_JOB을 실행한다 (spec §5.1.1 / §5.2.1).
//
// 정책:
//   - timeout_seconds > 0 이면 ctx.WithTimeout으로 강제 중단.
//   - timeout_seconds == 0이면 외부 ctx에만 의존 (데모 한정).
//   - stdout/stderr는 output_cap_bytes 안에서만 보존하고 초과분은 drop +
//     truncated=true (spec §5.2.1).
//   - 결과 분류: ctx deadline → TIMEOUT, exit 0 → SUCCESS, 그 외 → FAIL.
type ScriptRunner struct{}

func NewScriptRunner() *ScriptRunner {
	return &ScriptRunner{}
}

func (r *ScriptRunner) Run(ctx context.Context, agentID string, cmd model.Command) (model.JobResult, model.Target, error) {
	var spec model.ScriptJobSpec
	if err := json.Unmarshal(cmd.Spec, &spec); err != nil {
		return failResult(agentID, cmd, "invalid SCRIPT_JOB spec: "+err.Error()), scriptTarget(""), nil
	}

	target := scriptTarget(spec.ScriptPath)
	started := time.Now().UTC()

	runCtx := ctx
	var cancel context.CancelFunc
	if spec.TimeoutSeconds > 0 {
		runCtx, cancel = context.WithTimeout(ctx, time.Duration(spec.TimeoutSeconds)*time.Second)
		defer cancel()
	}

	stdout := NewCapBuffer(spec.OutputCapBytes)
	stderr := NewCapBuffer(spec.OutputCapBytes)

	process := exec.CommandContext(runCtx, spec.ScriptPath, spec.Args...)
	process.Stdout = stdout
	process.Stderr = stderr

	runErr := process.Run()
	finished := time.Now().UTC()

	status, exitCode := classifyScriptRun(runCtx, runErr, process.ProcessState)
	truncated := stdout.Truncated() || stderr.Truncated()

	return model.JobResult{
		ExecutionID: cmd.ExecutionID,
		ScheduleID:  cmd.ScheduleID,
		JobID:       cmd.JobID,
		AgentID:     agentID,
		JobType:     model.JobTypeScript,
		Status:      status,
		StartedAt:   started,
		FinishedAt:  finished,
		Script: &model.ScriptResult{
			ExitCode:  exitCode,
			StdoutCap: stdout.String(),
			StderrCap: stderr.String(),
			Truncated: truncated,
		},
	}, target, nil
}

// classifyScriptRun은 exec.Cmd.Run 결과를 spec §5.2 status로 분류한다.
// ctx deadline은 다른 어떤 신호보다 우선 — kill된 프로세스의 exit code는
// 의미가 없으므로 무시하고 TIMEOUT으로 보고.
func classifyScriptRun(runCtx context.Context, runErr error, ps *os.ProcessState) (model.JobStatus, int) {
	if errors.Is(runCtx.Err(), context.DeadlineExceeded) {
		exit := -1
		if ps != nil {
			exit = ps.ExitCode()
		}
		return model.JobStatusTimeout, exit
	}

	if runErr == nil {
		return model.JobStatusSuccess, 0
	}

	var exitErr *exec.ExitError
	if errors.As(runErr, &exitErr) {
		return model.JobStatusFail, exitErr.ExitCode()
	}

	// 스크립트 파일 부재 / 권한 거부 등 exec 자체 실패.
	return model.JobStatusFail, -1
}

func scriptTarget(path string) model.Target {
	return model.Target{Type: model.TargetTypeScript, ID: path}
}

// failResult는 spec 형식을 유지한 채 FAIL 결과를 만든다.
// stderr에 사유 문자열을 담아 BE/사용자가 원인 파악할 수 있게 한다.
func failResult(agentID string, cmd model.Command, stderrMsg string) model.JobResult {
	now := time.Now().UTC()
	return model.JobResult{
		ExecutionID: cmd.ExecutionID,
		ScheduleID:  cmd.ScheduleID,
		JobID:       cmd.JobID,
		AgentID:     agentID,
		JobType:     model.JobTypeScript,
		Status:      model.JobStatusFail,
		StartedAt:   now,
		FinishedAt:  now,
		Script: &model.ScriptResult{
			ExitCode:  -1,
			StderrCap: stderrMsg,
		},
	}
}
