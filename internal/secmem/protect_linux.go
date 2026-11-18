//go:build linux

package secmem

import "golang.org/x/sys/unix"

func platformProtect(p []byte) {
	if err := unix.Madvise(p, unix.MADV_DONTDUMP); err != nil {
		record(err)
	}
}
