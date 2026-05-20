package job

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"monitoring/script-agent/internal/model"
)

func writeTempLog(t *testing.T, dir, name, contents string) string {
	t.Helper()
	p := filepath.Join(dir, name)
	if err := os.WriteFile(p, []byte(contents), 0o644); err != nil {
		t.Fatalf("write %s: %v", p, err)
	}
	return p
}

func buildLogCmd(t *testing.T, jobID, logPath, pattern string) model.Command {
	t.Helper()
	specJSON, err := json.Marshal(model.LogJobSpec{
		LogPath:  logPath,
		Pattern:  pattern,
		Encoding: "UTF-8",
	})
	if err != nil {
		t.Fatalf("marshal spec: %v", err)
	}
	return model.Command{
		ExecutionID:   "e-" + jobID,
		ScheduleID:    "s-" + jobID,
		JobID:         jobID,
		TargetAgentID: "agent-1",
		JobType:       model.JobTypeLog,
		Spec:          specJSON,
	}
}

// TestLogRunner_FirstRunStartsAtEnd: 첫 실행은 파일 끝부터 — 기존 라인은
// 카운트되지 않고, 이후 추가된 라인만 매칭한다.
func TestLogRunner_FirstRunStartsAtEnd(t *testing.T) {
	dir := t.TempDir()
	stateDir := filepath.Join(dir, "state")
	logPath := writeTempLog(t, dir, "app.log",
		"[2026-05-19 13:59:42] INFO Starting\n"+
			"[2026-05-19 13:59:43] ERROR Failed\n"+
			"[2026-05-19 13:59:55] ERROR Retry\n",
	)
	cmd := buildLogCmd(t, "job-1", logPath, "ERROR")

	r := NewLogRunner(stateDir)

	// 1차: 첫 실행 — 파일 끝부터.
	result, target, err := r.Run(context.Background(), "agent-1", cmd)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if result.Status != model.JobStatusSuccess {
		t.Errorf("Status = %q", result.Status)
	}
	if result.Log.MatchedLinesCount != 0 {
		t.Errorf("first run matched = %d, want 0 (tail behavior)", result.Log.MatchedLinesCount)
	}
	if target.Type != model.TargetTypeLogFile || target.ID != logPath {
		t.Errorf("target = %+v", target)
	}

	// 라인 추가 → 2차는 추가된 ERROR만 카운트.
	f, err := os.OpenFile(logPath, os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		t.Fatalf("open append: %v", err)
	}
	f.WriteString("[2026-05-19 14:00:00] ERROR New failure\n")
	f.WriteString("[2026-05-19 14:00:01] INFO OK\n")
	f.WriteString("[2026-05-19 14:00:02] ERROR Another\n")
	f.Close()

	result2, _, err := r.Run(context.Background(), "agent-1", cmd)
	if err != nil {
		t.Fatalf("Run 2: %v", err)
	}
	if result2.Log.MatchedLinesCount != 2 {
		t.Errorf("second run matched = %d, want 2", result2.Log.MatchedLinesCount)
	}
	if len(result2.Log.SampleLines) != 2 {
		t.Errorf("samples len = %d, want 2", len(result2.Log.SampleLines))
	}
}

// TestLogRunner_SampleCapAt10: 매칭이 cap을 넘어도 samples는 10개로 제한.
func TestLogRunner_SampleCapAt10(t *testing.T) {
	dir := t.TempDir()
	stateDir := filepath.Join(dir, "state")
	logPath := writeTempLog(t, dir, "app.log", "")
	cmd := buildLogCmd(t, "job-cap", logPath, "ERROR")

	r := NewLogRunner(stateDir)
	// 1차는 비어있으니 끝부터(offset=0). state 저장.
	if _, _, err := r.Run(context.Background(), "agent-1", cmd); err != nil {
		t.Fatalf("Run 1: %v", err)
	}

	// 15개 ERROR 라인 추가
	f, _ := os.OpenFile(logPath, os.O_APPEND|os.O_WRONLY, 0o644)
	for i := 0; i < 15; i++ {
		f.WriteString("ERROR line\n")
	}
	f.Close()

	result, _, err := r.Run(context.Background(), "agent-1", cmd)
	if err != nil {
		t.Fatalf("Run 2: %v", err)
	}
	if result.Log.MatchedLinesCount != 15 {
		t.Errorf("matched = %d, want 15", result.Log.MatchedLinesCount)
	}
	if len(result.Log.SampleLines) != logSampleLinesMax {
		t.Errorf("samples len = %d, want %d", len(result.Log.SampleLines), logSampleLinesMax)
	}
}

// TestLogRunner_UnsupportedEncodingFails: UTF-8 외 encoding은 FAIL.
func TestLogRunner_UnsupportedEncodingFails(t *testing.T) {
	dir := t.TempDir()
	stateDir := filepath.Join(dir, "state")
	logPath := writeTempLog(t, dir, "app.log", "x")

	specJSON, _ := json.Marshal(model.LogJobSpec{
		LogPath:  logPath,
		Pattern:  ".",
		Encoding: "EUC-KR",
	})
	cmd := model.Command{
		ExecutionID: "e", ScheduleID: "s", JobID: "job-enc",
		TargetAgentID: "agent-1", JobType: model.JobTypeLog,
		Spec: specJSON,
	}

	r := NewLogRunner(stateDir)
	result, _, _ := r.Run(context.Background(), "agent-1", cmd)
	if result.Status != model.JobStatusFail {
		t.Errorf("Status = %q, want FAIL", result.Status)
	}
}

// TestLogRunner_InvalidPatternFails: 컴파일 안 되는 regex는 FAIL.
func TestLogRunner_InvalidPatternFails(t *testing.T) {
	dir := t.TempDir()
	stateDir := filepath.Join(dir, "state")
	logPath := writeTempLog(t, dir, "app.log", "x")
	cmd := buildLogCmd(t, "job-bad", logPath, "[unclosed")

	r := NewLogRunner(stateDir)
	result, _, _ := r.Run(context.Background(), "agent-1", cmd)
	if result.Status != model.JobStatusFail {
		t.Errorf("Status = %q, want FAIL", result.Status)
	}
}

// TestLogRunner_TruncateResetsAndCountsFromZero: 파일이 truncate되면
// state.Offset > currentSize 가 되고 0부터 다시 읽는다.
func TestLogRunner_TruncateResetsAndCountsFromZero(t *testing.T) {
	dir := t.TempDir()
	stateDir := filepath.Join(dir, "state")
	logPath := writeTempLog(t, dir, "app.log", "ERROR a\nINFO b\nERROR c\n")
	cmd := buildLogCmd(t, "job-trunc", logPath, "ERROR")

	r := NewLogRunner(stateDir)
	// 1차: 첫 실행 (파일 끝에서 시작), 매칭 0.
	if _, _, err := r.Run(context.Background(), "agent-1", cmd); err != nil {
		t.Fatalf("Run 1: %v", err)
	}

	// truncate (size 0으로) + 새 라인 추가
	if err := os.WriteFile(logPath, []byte("ERROR x\nERROR y\n"), 0o644); err != nil {
		t.Fatalf("truncate write: %v", err)
	}

	result, _, err := r.Run(context.Background(), "agent-1", cmd)
	if err != nil {
		t.Fatalf("Run 2: %v", err)
	}
	if result.Log.MatchedLinesCount != 2 {
		t.Errorf("after truncate matched = %d, want 2", result.Log.MatchedLinesCount)
	}
}
