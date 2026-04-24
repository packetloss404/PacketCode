package procrun

// BoundedBuffer keeps the first limit bytes written to it and discards the
// rest. It is intended for command output where retaining unbounded stdout or
// stderr could exhaust memory.
type BoundedBuffer struct {
	buf       []byte
	limit     int
	truncated bool
}

func NewBoundedBuffer(limit int) *BoundedBuffer {
	return &BoundedBuffer{limit: limit}
}

func (b *BoundedBuffer) Write(p []byte) (int, error) {
	if b.limit <= 0 {
		if len(p) > 0 {
			b.truncated = true
		}
		return len(p), nil
	}
	if remaining := b.limit - len(b.buf); remaining > 0 {
		if len(p) <= remaining {
			b.buf = append(b.buf, p...)
		} else {
			b.buf = append(b.buf, p[:remaining]...)
			b.truncated = true
		}
	} else if len(p) > 0 {
		b.truncated = true
	}
	return len(p), nil
}

func (b *BoundedBuffer) Bytes() []byte { return b.buf }

func (b *BoundedBuffer) String() string { return string(b.buf) }

func (b *BoundedBuffer) Truncated() bool { return b.truncated }
