// Package identity는 Agent의 영구 식별자(agent_id)를 관리한다.
//
// spec v0.2.1 §3.1:
//   - Agent 첫 실행 시 지정 경로의 파일을 확인.
//   - 파일이 없으면 UUIDv4를 생성해 저장.
//   - 파일이 있으면 그 값을 사용.
//   - agent_id는 hostname/OS 변경과 무관한 Agent의 영구 식별자.
package identity

import (
	"errors"
	"fmt"
	"os"
	"strings"

	"github.com/google/uuid"
)

// GetOrCreate path 위치의 파일에서 agent_id를 로드하거나,
// 없으면 UUIDv4를 생성해 저장한 뒤 반환한다.
//
// 파일이 존재하지만 내용이 비어있는 경우는 식별자가 유실된 비정상
// 상태로 보고 에러를 반환한다(silent regenerate는 영구 식별자
// 의미를 깨므로 금지).
func GetOrCreate(path string) (string, error) {
	data, err := os.ReadFile(path)
	if err == nil {
		id := strings.TrimSpace(string(data))
		if id == "" {
			return "", fmt.Errorf("identity: agent_id file %q exists but is empty", path)
		}
		return id, nil
	}
	if !errors.Is(err, os.ErrNotExist) {
		return "", fmt.Errorf("identity: read %q: %w", path, err)
	}

	// 신규 발급.
	id := uuid.NewString()
	if err := os.WriteFile(path, []byte(id), 0o644); err != nil {
		return "", fmt.Errorf("identity: write %q: %w", path, err)
	}
	return id, nil
}
