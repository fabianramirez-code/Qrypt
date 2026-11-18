package secmem

import (
	"errors"
	"sync"
	"testing"
)

func TestBufferLifetime(t *testing.T) {
	b := New(65)
	if len(b.Bytes()) != 65 {
		t.Fatal("length")
	}
	b.Bytes()[64] = 99
	if b.Bytes()[64] != 99 {
		t.Fatal("write lost")
	}
	b.Destroy()
	if b.Bytes() != nil || b.Locked() {
		t.Fatal("destroyed buffer accessible")
	}
	b.Destroy()
	var empty *Buffer
	empty.Destroy()
	if empty.Bytes() != nil || empty.Locked() {
		t.Fatal("nil receiver")
	}
}

func TestRefusals(t *testing.T) {
	oldLock, oldAlloc := lockPages, allocPages
	t.Cleanup(func() { lockPages, allocPages = oldLock, oldAlloc })
	lockPages = func([]byte) error { return errors.New("simulated lock refusal") }
	b := New(32)
	if b.Locked() || len(b.Bytes()) != 32 {
		t.Fatal("lock refusal broke buffer")
	}
	b.Bytes()[0] = 4
	b.Destroy()
	if ok, why := Status(); ok || why == "" {
		t.Fatalf("status: %v %q", ok, why)
	}
	allocPages = func(int) ([]byte, error) { return nil, errors.New("simulated allocation refusal") }
	fallback := New(32)
	if fallback.Locked() || len(fallback.Bytes()) != 32 {
		t.Fatal("heap fallback failed")
	}
	fallback.Bytes()[0] = 7
	held := fallback.Bytes()
	fallback.Destroy()
	if held[0] != 0 {
		t.Fatal("heap fallback not wiped before release")
	}
}

func TestManyAndConcurrent(t *testing.T) {
	buffers := make([]*Buffer, 1024)
	locked := 0
	for i := range buffers {
		buffers[i] = New(32)
		if len(buffers[i].Bytes()) != 32 {
			t.Fatal("allocation failed")
		}
		if buffers[i].Locked() {
			locked++
		}
	}
	t.Logf("locked %d/1024", locked)
	for _, b := range buffers {
		b.Destroy()
	}
	var wg sync.WaitGroup
	for i := 0; i < 16; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for n := 0; n < 64; n++ {
				b := New(32)
				b.Bytes()[0] = 1
				b.Destroy()
			}
		}()
	}
	wg.Wait()
}
