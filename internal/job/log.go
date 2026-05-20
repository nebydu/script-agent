package job

import (
	"bufio"
	"context"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"monitoring/script-agent/internal/model"
)

// logSampleLinesMax는 결과 페이로드에 포함하는 매칭 라인의 최대 개수다.
// 결과 크기 제어용 (사전 결정).
const logSampleLinesMax = 10

// LogRunner는 LOG_JOB을 실행한다 (spec §5.1.2 / §5.2.2).
//
// 정책:
//   - 첫 실행 (file_state 없음) → 파일 끝부터 매칭 (tail -f 스타일).
//     초기 결과는 matched=0, samples=[].
//   - rotation 감지 (file_id 변경 또는 size shrink) → 새 파일을 처음부터.
//   - encoding은 UTF-8만 (데모). 그 외는 FAIL.
//   - file_state는 stateDir 아래 <job_id>.json으로 영속.
type LogRunner struct {
	stateDir string
}

func NewLogRunner(stateDir string) *LogRunner {
	if stateDir == "" {
		stateDir = "./.agent_state"
	}
	return &LogRunner{stateDir: stateDir}
}

func (r *LogRunner) Run(ctx context.Context, agentID string, cmd model.Command) (model.JobResult, model.Target, error) {
	var spec model.LogJobSpec
	if err := json.Unmarshal(cmd.Spec, &spec); err != nil {
		return logFail(agentID, cmd, "invalid LOG_JOB spec: "+err.Error()), logTarget(""), nil
	}

	target := logTarget(spec.LogPath)
	started := time.Now().UTC()

	if spec.Encoding != "" && !strings.EqualFold(spec.Encoding, "UTF-8") {
		return logFail(agentID, cmd, "unsupported encoding: "+spec.Encoding), target, nil
	}

	re, err := regexp.Compile(spec.Pattern)
	if err != nil {
		return logFail(agentID, cmd, "invalid pattern: "+err.Error()), target, nil
	}

	f, err := os.Open(spec.LogPath)
	if err != nil {
		return logFail(agentID, cmd, "open log file: "+err.Error()), target, nil
	}
	defer f.Close()

	stat, err := f.Stat()
	if err != nil {
		return logFail(agentID, cmd, "stat log file: "+err.Error()), target, nil
	}
	currentSize := stat.Size()

	currentFileID, err := extractFileID(f)
	if err != nil {
		return logFail(agentID, cmd, "extract file_id: "+err.Error()), target, nil
	}

	statePath := filepath.Join(r.stateDir, cmd.JobID+".json")
	state, hasState, err := loadFileState(statePath)
	if err != nil {
		return logFail(agentID, cmd, "load file_state: "+err.Error()), target, nil
	}

	readFrom := decideReadFrom(hasState, state, currentSize, currentFileID)

	if _, err := f.Seek(readFrom, io.SeekStart); err != nil {
		return logFail(agentID, cmd, "seek: "+err.Error()), target, nil
	}

	matchedCount, samples, scanErr := scanForPattern(ctx, f, re)
	// scan 도중 ctx 취소되면 부분 결과 그대로 진행 (state는 진행한 만큼만 저장).
	if scanErr != nil && ctx.Err() == nil {
		// 실제 read 에러
		return logFail(agentID, cmd, "scan: "+scanErr.Error()), target, nil
	}

	// 새 offset = 현재 read 위치. scanner 내부 buffer 때문에 정확하지 않을 수
	// 있으나, Seek(0, Current)로 OS-level file pointer를 가져옴.
	newOffset, err := f.Seek(0, io.SeekCurrent)
	if err != nil {
		newOffset = currentSize
	}

	newState := FileState{
		Offset: newOffset,
		Size:   currentSize,
		FileID: currentFileID,
	}
	// state 저장 실패는 결과 자체를 망치지 않음 — 다음 실행에서 first run으로
	// 다시 시작되거나 동일 라인 재카운트 가능. 데모 단순화 (본개발에서 audit 검토).
	_ = saveFileState(statePath, newState)

	finished := time.Now().UTC()
	return model.JobResult{
		ExecutionID: cmd.ExecutionID,
		ScheduleID:  cmd.ScheduleID,
		JobID:       cmd.JobID,
		AgentID:     agentID,
		JobType:     model.JobTypeLog,
		Status:      model.JobStatusSuccess,
		StartedAt:   started,
		FinishedAt:  finished,
		Log: &model.LogResult{
			MatchedLinesCount: matchedCount,
			SampleLines:       samples,
		},
	}, target, nil
}

// scanForPattern은 reader에서 라인 단위로 읽으면서 regex 매칭 라인 수와
// 샘플(최대 logSampleLinesMax)을 모은다.
func scanForPattern(ctx context.Context, rd io.Reader, re *regexp.Regexp) (int, []string, error) {
	scanner := bufio.NewScanner(rd)
	matched := 0
	var samples []string
	for scanner.Scan() {
		// 큰 파일에서 ctx 취소를 빠르게 반영.
		if err := ctx.Err(); err != nil {
			return matched, samples, nil
		}
		line := scanner.Text()
		if re.MatchString(line) {
			matched++
			if len(samples) < logSampleLinesMax {
				samples = append(samples, line)
			}
		}
	}
	return matched, samples, scanner.Err()
}

func logTarget(path string) model.Target {
	return model.Target{Type: model.TargetTypeLogFile, ID: path}
}

func logFail(agentID string, cmd model.Command, msg string) model.JobResult {
	now := time.Now().UTC()
	return model.JobResult{
		ExecutionID: cmd.ExecutionID,
		ScheduleID:  cmd.ScheduleID,
		JobID:       cmd.JobID,
		AgentID:     agentID,
		JobType:     model.JobTypeLog,
		Status:      model.JobStatusFail,
		StartedAt:   now,
		FinishedAt:  now,
		Log: &model.LogResult{
			SampleLines: []string{msg},
		},
	}
}
