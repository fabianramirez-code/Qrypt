package secmem

import (
	"sync"
	"testing"
)

func TestDestroyFromSeveralGoroutinesFreesOnce(t *testing.T) {
	before := Live()
	b := New(64)
	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() { defer wg.Done(); b.Destroy() }()
	}
	wg.Wait()
	if got := Live(); got != before {
		t.Fatalf("live buffers = %d, want %d", got, before)
	}
}
