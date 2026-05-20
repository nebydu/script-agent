package job

import (
	"path/filepath"
	"testing"
)

func TestFileState_SaveLoadRoundTrip(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "job-1.json")

	in := FileState{Offset: 1234, Size: 5678, FileID: "abc-123"}
	if err := saveFileState(path, in); err != nil {
		t.Fatalf("save: %v", err)
	}
	out, has, err := loadFileState(path)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if !has {
		t.Fatal("hasState should be true")
	}
	if out != in {
		t.Errorf("roundtrip mismatch: got=%+v want=%+v", out, in)
	}
}

func TestFileState_LoadMissingReturnsNoState(t *testing.T) {
	dir := t.TempDir()
	_, has, err := loadFileState(filepath.Join(dir, "nope.json"))
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if has {
		t.Errorf("hasState should be false for missing file")
	}
}

func TestFileState_SaveCreatesStateDir(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "nested", "subdir", "job.json")
	if err := saveFileState(path, FileState{Offset: 1, Size: 1, FileID: "x"}); err != nil {
		t.Fatalf("save: %v", err)
	}
	_, has, _ := loadFileState(path)
	if !has {
		t.Errorf("file should exist after save")
	}
}

func TestDecideReadFrom_FirstRunStartsAtEnd(t *testing.T) {
	got := decideReadFrom(false, FileState{}, 1000, "id-1")
	if got != 1000 {
		t.Errorf("first run readFrom = %d, want 1000 (file end)", got)
	}
}

func TestDecideReadFrom_RotationResetsToZero(t *testing.T) {
	state := FileState{Offset: 500, Size: 1000, FileID: "old-id"}
	got := decideReadFrom(true, state, 200, "new-id")
	if got != 0 {
		t.Errorf("rotation (file_id changed) readFrom = %d, want 0", got)
	}
}

func TestDecideReadFrom_TruncateResetsToZero(t *testing.T) {
	state := FileState{Offset: 500, Size: 500, FileID: "id-1"}
	got := decideReadFrom(true, state, 100, "id-1") // same id, size shrunk
	if got != 0 {
		t.Errorf("truncate (size < offset) readFrom = %d, want 0", got)
	}
}

func TestDecideReadFrom_NormalContinues(t *testing.T) {
	state := FileState{Offset: 500, Size: 500, FileID: "id-1"}
	got := decideReadFrom(true, state, 700, "id-1")
	if got != 500 {
		t.Errorf("normal readFrom = %d, want 500 (state offset)", got)
	}
}
