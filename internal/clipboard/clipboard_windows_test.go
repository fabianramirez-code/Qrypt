//go:build windows

package clipboard_test

import (
	"crypto/rand"
	"encoding/hex"
	"runtime"
	"testing"
	"time"
	"unsafe"

	"github.com/fabianramirez-code/qrypt/internal/clipboard"
	"golang.org/x/sys/windows"
)

var (
	user32           = windows.NewLazySystemDLL("user32.dll")
	openClipboard    = user32.NewProc("OpenClipboard")
	closeClipboard   = user32.NewProc("CloseClipboard")
	getClipboardData = user32.NewProc("GetClipboardData")
	getDesktopWindow = user32.NewProc("GetDesktopWindow")
	kernel32         = windows.NewLazySystemDLL("kernel32.dll")
	globalLock       = kernel32.NewProc("GlobalLock")
	globalUnlock     = kernel32.NewProc("GlobalUnlock")
)

func windowsPointer(address uintptr) unsafe.Pointer {
	return *(*unsafe.Pointer)(unsafe.Pointer(&address))
}

func clipboardText(t *testing.T) string {
	t.Helper()
	hwnd, _, _ := getDesktopWindow.Call()
	var opened uintptr
	for attempt := 0; attempt < 10; attempt++ {
		opened, _, _ = openClipboard.Call(hwnd)
		if opened != 0 {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	if opened == 0 {
		t.Fatal("could not open clipboard for inspection")
	}
	defer closeClipboard.Call()
	h, _, _ := getClipboardData.Call(13) // CF_UNICODETEXT
	if h == 0 {
		return ""
	}
	p, _, _ := globalLock.Call(h)
	if p == 0 {
		t.Fatal("could not lock clipboard data")
	}
	defer globalUnlock.Call(h)
	return windows.UTF16PtrToString((*uint16)(windowsPointer(p)))
}

func TestCopyAndClearIfUnchanged(t *testing.T) {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		t.Fatal(err)
	}
	text := "qrypt-clipboard-test-" + hex.EncodeToString(b[:])
	if err := clipboard.Copy(text); err != nil {
		t.Fatal(err)
	}
	defer clipboard.ClearIfUnchanged(text)
	if got := clipboardText(t); got != text {
		t.Fatal("copy did not store test text")
	}
	if err := clipboard.ClearIfUnchanged(text + "-different"); err != nil {
		t.Fatal(err)
	}
	if got := clipboardText(t); got != text {
		t.Fatal("mismatched clear changed clipboard")
	}
	if err := clipboard.ClearIfUnchanged(text); err != nil {
		t.Fatal(err)
	}
	if got := clipboardText(t); got != "" {
		t.Fatal("matching clear did not empty clipboard")
	}
}
