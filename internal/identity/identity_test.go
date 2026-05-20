package identity

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/google/uuid"
)

// TestGetOrCreate_NewFile: 파일이 없을 때 UUIDv4를 생성해 저장하고
// 그 값을 반환하는지 확인.
func TestGetOrCreate_NewFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), ".agent_id")

	id, err := GetOrCreate(path)
	if err != nil {
		t.Fatalf("GetOrCreate returned error: %v", err)
	}

	// 반환값이 유효한 UUID인지 검증.
	if _, err := uuid.Parse(id); err != nil {
		t.Fatalf("returned id %q is not a valid UUID: %v", id, err)
	}

	// 파일이 생성되었고 내용이 반환값과 일치하는지 검증.
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("expected file %q to exist after GetOrCreate: %v", path, err)
	}
	if got := string(data); got != id {
		t.Fatalf("file content mismatch: got %q, want %q", got, id)
	}
}

// TestGetOrCreate_ExistingFile: 파일이 이미 존재하면 그 값을 그대로
// 반환하고, 두 번 호출해도 동일 값이 유지되는지 확인 (영구 식별자
// 보장 — spec §3.1).
func TestGetOrCreate_ExistingFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), ".agent_id")

	first, err := GetOrCreate(path)
	if err != nil {
		t.Fatalf("first GetOrCreate failed: %v", err)
	}

	// 첫 생성 시점의 파일 메타데이터 캡처.
	infoBefore, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat before: %v", err)
	}

	second, err := GetOrCreate(path)
	if err != nil {
		t.Fatalf("second GetOrCreate failed: %v", err)
	}

	if first != second {
		t.Fatalf("agent_id changed across calls: first=%q second=%q", first, second)
	}

	// 두 번째 호출이 파일을 다시 쓰지 않았는지 확인 (mtime 보존).
	infoAfter, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat after: %v", err)
	}
	if !infoBefore.ModTime().Equal(infoAfter.ModTime()) {
		t.Fatalf("file was rewritten on second call: mtime changed from %v to %v",
			infoBefore.ModTime(), infoAfter.ModTime())
	}
}
