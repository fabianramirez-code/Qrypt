// Package secmem holds long-lived key bytes in locked, page-sized allocations when possible.
// Failure to allocate or lock degrades safely to usable (but less protected) memory.
package secmem

import (
	"fmt"
	"os"
	"sync"
	"sync/atomic"
)

// Buffer owns a writable byte slice. Do not retain Bytes after Destroy.
type Buffer struct {
	data   []byte
	pages  []byte
	locked bool
}

var (
	allocPages   = platformAlloc
	lockPages    = platformLock
	statusMu     sync.Mutex
	firstFailure string
	live         atomic.Int64
)

func record(err error) {
	if err == nil {
		return
	}
	statusMu.Lock()
	if firstFailure == "" {
		firstFailure = err.Error()
	}
	statusMu.Unlock()
}

// Status reports whether all page protections attempted so far succeeded.
func Status() (locked bool, reason string) {
	statusMu.Lock()
	defer statusMu.Unlock()
	return firstFailure == "", firstFailure
}

// Live reports the number of buffers not yet destroyed.
func Live() int64 { return live.Load() }

// SimulateFailureForTest injects page allocation or locking refusal until the
// returned restore function is called. Tests must not use it concurrently.
func SimulateFailureForTest(allocation, locking bool) func() {
	originalAlloc, originalLock := allocPages, lockPages
	if allocation {
		allocPages = func(int) ([]byte, error) { return nil, fmt.Errorf("simulated page allocation refusal") }
	}
	if locking {
		lockPages = func([]byte) error { return fmt.Errorf("simulated page lock refusal") }
	}
	return func() { allocPages, lockPages = originalAlloc, originalLock }
}

// MaxSize bounds allocations to one MiB. Larger requests return an empty buffer
// and record a failure rather than panicking or allocating an unbounded heap slice.
const MaxSize = 1 << 20

// New creates a buffer of size bytes. Page protection failures never stop the caller.
func New(size int) *Buffer {
	if size > MaxSize {
		record(fmt.Errorf("page allocation: size exceeds %d bytes", MaxSize))
		return &Buffer{}
	}
	if size < 0 {
		size = 0
	}
	b := &Buffer{}
	page := os.Getpagesize()
	if size > 0 && size <= int(^uint(0)>>1)-(page-1) {
		count := (size + page - 1) / page * page
		pages, err := allocPages(count)
		if err == nil && len(pages) >= count {
			b.pages = pages
			b.data = pages[:size]
			if err := lockPages(pages); err != nil {
				record(fmt.Errorf("page lock: %w", err))
			} else {
				b.locked = true
			}
			platformProtect(pages)
		} else if err != nil {
			record(fmt.Errorf("page allocation: %w", err))
		} else {
			record(fmt.Errorf("page allocation: short mapping"))
		}
	} else if size > 0 {
		record(fmt.Errorf("page allocation: size overflow"))
	}
	if b.pages == nil {
		b.data = make([]byte, size)
	}
	live.Add(1)
	return b
}

// Bytes returns the usable slice, or nil after Destroy.
func (b *Buffer) Bytes() []byte {
	if b == nil {
		return nil
	}
	return b.data
}

// Locked reports whether this buffer is currently locked against paging.
func (b *Buffer) Locked() bool { return b != nil && b.locked }

// Destroy wipes the entire allocation before releasing it. It is nil-safe and idempotent.
func (b *Buffer) Destroy() {
	if b == nil || b.data == nil && b.pages == nil {
		return
	}
	if b.pages != nil {
		clear(b.pages)
		if b.locked {
			platformUnlock(b.pages)
		}
		platformFree(b.pages)
	} else {
		clear(b.data)
	}
	b.data, b.pages, b.locked = nil, nil, false
	live.Add(-1)
}
