//go:build windows

package secmem

import (
	"sync"
	"unsafe"

	"golang.org/x/sys/windows"
)

var workingSetOnce sync.Once

func platformAlloc(n int) ([]byte, error) {
	addr, err := windows.VirtualAlloc(0, uintptr(n), windows.MEM_RESERVE|windows.MEM_COMMIT, windows.PAGE_READWRITE)
	if err != nil {
		return nil, err
	}
	return unsafe.Slice((*byte)(unsafe.Add(nil, addr)), n), nil
}
func platformLock(p []byte) error {
	workingSetOnce.Do(func() {
		var min, max uintptr
		var flags uint32
		windows.GetProcessWorkingSetSizeEx(windows.CurrentProcess(), &min, &max, &flags)
		// Keep the original maximum at least as large as the new minimum.
		const target = 16 << 20
		if min < target {
			if max < target {
				max = target
			}
			if err := windows.SetProcessWorkingSetSizeEx(windows.CurrentProcess(), target, max, flags); err != nil {
				record(err)
			}
		}
	})
	return windows.VirtualLock(uintptr(unsafe.Pointer(&p[0])), uintptr(len(p)))
}
func platformProtect([]byte) {}
func platformUnlock(p []byte) {
	_ = windows.VirtualUnlock(uintptr(unsafe.Pointer(&p[0])), uintptr(len(p)))
}
func platformFree(p []byte) {
	_ = windows.VirtualFree(uintptr(unsafe.Pointer(&p[0])), 0, windows.MEM_RELEASE)
}
