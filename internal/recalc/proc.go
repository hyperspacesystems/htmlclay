package recalc

import (
	"bytes"
	"sync"
)

type limitedBuffer struct {
	mu  sync.Mutex
	buf bytes.Buffer
	max int
}

func (b *limitedBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.buf.Len() < b.max {
		rem := b.max - b.buf.Len()
		if len(p) <= rem {
			b.buf.Write(p)
		} else {
			b.buf.Write(p[:rem])
		}
	}
	return len(p), nil
}

func (b *limitedBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.String()
}
