package secmem

import (
	"os"
	"testing"

	"golang.org/x/sys/windows"
)

func TestWorkingSetLimits(t *testing.T) {
	var minimum, maximum uintptr
	var flags uint32
	windows.GetProcessWorkingSetSizeEx(windows.CurrentProcess(), &minimum, &maximum, &flags)
	t.Logf("working set before: minimum=%d maximum=%d", minimum, maximum)
	count := func() int {
		pages := make([]uintptr, 0, 4096)
		defer func() {
			for _, addr := range pages {
				_ = windows.VirtualUnlock(addr, uintptr(os.Getpagesize()))
				_ = windows.VirtualFree(addr, 0, windows.MEM_RELEASE)
			}
		}()
		for len(pages) < 4096 {
			addr, err := windows.VirtualAlloc(0, uintptr(os.Getpagesize()), windows.MEM_COMMIT|windows.MEM_RESERVE, windows.PAGE_READWRITE)
			if err != nil {
				break
			}
			if err = windows.VirtualLock(addr, uintptr(os.Getpagesize())); err != nil {
				_ = windows.VirtualFree(addr, 0, windows.MEM_RELEASE)
				break
			}
			pages = append(pages, addr)
		}
		return len(pages)
	}
	before := count()
	t.Logf("one-page locks before adjustment: %d (cap 4096)", before)
	target := uintptr(16 << 20)
	if maximum < target {
		maximum = target
	}
	if err := windows.SetProcessWorkingSetSizeEx(windows.CurrentProcess(), target, maximum, flags); err != nil {
		t.Logf("adjustment refused: %v", err)
		return
	}
	defer func() { _ = windows.SetProcessWorkingSetSizeEx(windows.CurrentProcess(), minimum, maximum, flags) }()
	t.Logf("one-page locks after adjustment: %d (cap 4096)", count())
}
