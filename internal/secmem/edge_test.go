package secmem

import (
	"errors"
	"strings"
	"testing"
)

func TestEdgeSizes(t *testing.T) {
	for _, size := range []int{-5, 0, 1, 4095, 4096, 4097, 1 << 20} {
		b := New(size)
		want := max(0, size)
		if len(b.Bytes()) != want {
			t.Fatalf("size %d: got %d", size, len(b.Bytes()))
		}
		if want > 0 {
			b.Bytes()[want-1] = 42
		}
		b.Destroy()
		b.Destroy()
		if b.Bytes() != nil || b.Locked() {
			t.Fatalf("size %d: accessible after Destroy", size)
		}
	}
	var nilBuffer *Buffer
	nilBuffer.Destroy()
	var zero Buffer
	zero.Destroy()
	if nilBuffer.Bytes() != nil || nilBuffer.Locked() || zero.Bytes() != nil || zero.Locked() {
		t.Fatal("nil or zero buffer accessible")
	}
}

func TestEdgeFallbackAndFirstReason(t *testing.T) {
	originalAlloc, originalLock, originalReason := allocPages, lockPages, firstFailure
	t.Cleanup(func() { allocPages, lockPages, firstFailure = originalAlloc, originalLock, originalReason })
	firstFailure = ""
	allocPages = func(int) ([]byte, error) { return nil, errors.New("first refusal") }
	b := New(12)
	if b.Locked() || len(b.Bytes()) != 12 {
		t.Fatal("allocation fallback")
	}
	held := b.Bytes()
	held[0] = 8
	b.Destroy()
	if held[0] != 0 {
		t.Fatal("fallback not wiped")
	}
	allocPages = func(int) ([]byte, error) { return make([]byte, 1), nil }
	short := New(12)
	if short.Locked() || len(short.Bytes()) != 12 {
		t.Fatal("short mapping fallback")
	}
	short.Destroy()
	if ok, reason := Status(); ok || !strings.Contains(reason, "first refusal") {
		t.Fatalf("first reason: %t %q", ok, reason)
	}
}

func TestHugeSizeDoesNotPanic(t *testing.T) {
	b := New(1 << 62)
	defer b.Destroy()
	if b.Bytes() != nil {
		t.Fatal("oversized allocation has bytes")
	}
	if ok, reason := Status(); ok || reason == "" {
		t.Fatalf("status: %t %q", ok, reason)
	}
}
