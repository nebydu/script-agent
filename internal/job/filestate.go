package job

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
)

// FileState는 LOG_JOB이 한 로그 파일을 어디까지 읽었는지 추적하는
// Agent local state다 (spec §5.2.3 노트 — BE에 전송하지 않음).
//
// rotation 감지에 file_id(POSIX inode / Windows file index)와 size 모두 사용:
//   - FileID 변경 → 새 파일로 교체됨 (logrotate copy-truncate, rename 등).
//     offset 0으로 리셋, 새 파일을 처음부터 읽음.
//   - 같은 FileID + currentSize < state.Offset → in-place truncate.
//     offset 0으로 리셋.
//   - 그 외 → state.Offset에서 이어 읽기.
type FileState struct {
	Offset int64  `json:"offset"`
	Size   int64  `json:"size"`
	FileID string `json:"file_id"`
}

// loadFileState는 state 파일을 읽는다. 파일이 없으면 (false, nil) 반환.
func loadFileState(path string) (FileState, bool, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return FileState{}, false, nil
		}
		return FileState{}, false, fmt.Errorf("read %q: %w", path, err)
	}
	var st FileState
	if err := json.Unmarshal(data, &st); err != nil {
		return FileState{}, false, fmt.Errorf("unmarshal %q: %w", path, err)
	}
	return st, true, nil
}

// saveFileState는 state를 atomic하게 저장한다 (write-temp-then-rename).
// 디렉토리가 없으면 생성.
func saveFileState(path string, st FileState) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("mkdir state dir: %w", err)
	}
	data, err := json.Marshal(st)
	if err != nil {
		return fmt.Errorf("marshal: %w", err)
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o644); err != nil {
		return fmt.Errorf("write tmp: %w", err)
	}
	if err := os.Rename(tmp, path); err != nil {
		return fmt.Errorf("rename tmp: %w", err)
	}
	return nil
}

// decideReadFrom은 rotation/truncate 판단 + 시작 offset 결정 로직이다.
// LogRunner와 분리해 OS 의존 없이 단위 테스트한다.
//
//   - hasState=false → 첫 실행. 파일 끝부터 (currentSize). 모니터링 표준.
//   - state.FileID != currentFileID → rotation. offset 0.
//   - currentSize < state.Offset → truncate. offset 0.
//   - 그 외 → state.Offset.
func decideReadFrom(hasState bool, state FileState, currentSize int64, currentFileID string) int64 {
	if !hasState {
		return currentSize
	}
	if state.FileID != currentFileID {
		return 0
	}
	if currentSize < state.Offset {
		return 0
	}
	return state.Offset
}
