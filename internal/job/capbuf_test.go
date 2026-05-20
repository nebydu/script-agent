package job

import "testing"

func TestCapBuffer_UnderCap(t *testing.T) {
	b := NewCapBuffer(10)
	n, err := b.Write([]byte("hello"))
	if err != nil || n != 5 {
		t.Fatalf("Write = %d, %v", n, err)
	}
	if b.String() != "hello" {
		t.Errorf("String = %q", b.String())
	}
	if b.Truncated() {
		t.Errorf("Truncated should be false")
	}
}

func TestCapBuffer_ExactlyAtCap(t *testing.T) {
	b := NewCapBuffer(5)
	if _, err := b.Write([]byte("hello")); err != nil {
		t.Fatalf("Write: %v", err)
	}
	if b.Truncated() {
		t.Errorf("Truncated should be false when exactly at cap")
	}
}

func TestCapBuffer_OverflowInSingleWrite(t *testing.T) {
	b := NewCapBuffer(5)
	n, err := b.Write([]byte("hello world"))
	if err != nil || n != 11 {
		t.Fatalf("Write = %d, %v (Write must report full p len for exec compatibility)", n, err)
	}
	if b.String() != "hello" {
		t.Errorf("String = %q, want %q", b.String(), "hello")
	}
	if !b.Truncated() {
		t.Errorf("Truncated should be true")
	}
}

func TestCapBuffer_OverflowAcrossWrites(t *testing.T) {
	b := NewCapBuffer(5)
	b.Write([]byte("hel"))
	b.Write([]byte("lo!"))
	if b.String() != "hello" {
		t.Errorf("String = %q", b.String())
	}
	if !b.Truncated() {
		t.Errorf("Truncated should be true")
	}
	// 추가 Write도 Truncated 유지
	b.Write([]byte("more"))
	if !b.Truncated() {
		t.Errorf("Truncated should remain true")
	}
}

func TestCapBuffer_UnlimitedWhenCapZero(t *testing.T) {
	b := NewCapBuffer(0)
	b.Write([]byte("anything large"))
	if b.Truncated() {
		t.Errorf("cap=0 should be unlimited, Truncated should be false")
	}
	if b.String() != "anything large" {
		t.Errorf("String = %q", b.String())
	}
}
