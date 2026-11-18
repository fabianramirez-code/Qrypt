//go:build linux || darwin

package secmem

import "golang.org/x/sys/unix"

func platformAlloc(n int) ([]byte, error) {
	return unix.Mmap(-1, 0, n, unix.PROT_READ|unix.PROT_WRITE, unix.MAP_PRIVATE|unix.MAP_ANON)
}
func platformLock(p []byte) error { return unix.Mlock(p) }
func platformUnlock(p []byte)     { _ = unix.Munlock(p) }
func platformFree(p []byte)       { _ = unix.Munmap(p) }
