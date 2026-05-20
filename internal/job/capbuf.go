package job

import "sync"

// CapBuffer는 지정된 cap byte까지 누적하고 그 이후 Write는 drop하는
// io.Writer다. SCRIPT_JOB의 output_cap_bytes 정책 구현용 (spec §5.1.1).
//
// cap <= 0 이면 무제한 (drop 안 함, Truncated() 항상 false).
// cap > 0 이고 cap을 넘어선 Write가 한 번이라도 발생하면 Truncated() = true.
//
// Write는 항상 len(p), nil을 반환해 호출 측(exec.Cmd 등)이 정상으로 인식.
type CapBuffer struct {
	mu        sync.Mutex
	cap       int
	buf       []byte
	truncated bool
}

func NewCapBuffer(cap int) *CapBuffer {
	return &CapBuffer{cap: cap}
}

func (b *CapBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()

	if b.cap <= 0 {
		b.buf = append(b.buf, p...)
		return len(p), nil
	}

	remaining := b.cap - len(b.buf)
	if remaining <= 0 {
		b.truncated = true
		return len(p), nil
	}
	if len(p) <= remaining {
		b.buf = append(b.buf, p...)
		return len(p), nil
	}
	b.buf = append(b.buf, p[:remaining]...)
	b.truncated = true
	return len(p), nil
}

func (b *CapBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return string(b.buf)
}

func (b *CapBuffer) Truncated() bool {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.truncated
}

func (b *CapBuffer) Len() int {
	b.mu.Lock()
	defer b.mu.Unlock()
	return len(b.buf)
}
