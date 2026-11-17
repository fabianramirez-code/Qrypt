//go:build windows

package clipboard

import (
	"encoding/binary"
	"fmt"
	"syscall"
	"time"
	"unicode/utf16"
	"unsafe"

	"golang.org/x/sys/windows"
)

const (
	cfUnicodeText = 13
	gmemMoveable  = 0x0002
	gmemZeroInit  = 0x0040
)

var (
	user32            = windows.NewLazySystemDLL("user32.dll")
	kernel32          = windows.NewLazySystemDLL("kernel32.dll")
	openClipboard     = user32.NewProc("OpenClipboard")
	closeClipboard    = user32.NewProc("CloseClipboard")
	emptyClipboard    = user32.NewProc("EmptyClipboard")
	setClipboardData  = user32.NewProc("SetClipboardData")
	getClipboardData  = user32.NewProc("GetClipboardData")
	isFormatAvailable = user32.NewProc("IsClipboardFormatAvailable")
	registerFormat    = user32.NewProc("RegisterClipboardFormatW")
	getDesktopWindow  = user32.NewProc("GetDesktopWindow")
	globalAlloc       = kernel32.NewProc("GlobalAlloc")
	globalFree        = kernel32.NewProc("GlobalFree")
	globalLock        = kernel32.NewProc("GlobalLock")
	globalUnlock      = kernel32.NewProc("GlobalUnlock")
	globalSize        = kernel32.NewProc("GlobalSize")
)

// nativePointer converts an address returned by Win32 without a uintptr-to-pointer cast.
func nativePointer(address uintptr) unsafe.Pointer {
	return *(*unsafe.Pointer)(unsafe.Pointer(&address))
}

func winError(operation string, err error) error {
	if err == nil || err == syscall.Errno(0) {
		err = syscall.EINVAL
	}
	return fmt.Errorf("clipboard: %s: %w", operation, err)
}

func withClipboard(fn func() error) (err error) {
	hwnd, _, callErr := getDesktopWindow.Call()
	if hwnd == 0 {
		return winError("GetDesktopWindow", callErr)
	}
	for attempt := 0; attempt < 5; attempt++ {
		ok, _, openErr := openClipboard.Call(hwnd)
		if ok != 0 {
			defer func() {
				closed, _, closeErr := closeClipboard.Call()
				if closed == 0 && err == nil {
					err = winError("CloseClipboard", closeErr)
				}
			}()
			return fn()
		}
		if attempt == 4 {
			return winError("OpenClipboard", openErr)
		}
		time.Sleep(20 * time.Millisecond)
	}
	return nil
}

func registeredFormat(name string) (uintptr, error) {
	wide, err := windows.UTF16PtrFromString(name)
	if err != nil {
		return 0, err
	}
	id, _, callErr := registerFormat.Call(uintptr(unsafe.Pointer(wide)))
	if id == 0 {
		return 0, winError("RegisterClipboardFormatW", callErr)
	}
	return id, nil
}

// setBytes gives ownership of the movable block to Windows only on success.
func setBytes(format uintptr, data []byte) error {
	size := len(data)
	if size == 0 {
		size = 1 // A nil handle requests delayed rendering, not an empty format.
	}
	h, _, allocErr := globalAlloc.Call(gmemMoveable|gmemZeroInit, uintptr(size))
	if h == 0 {
		return winError("GlobalAlloc", allocErr)
	}
	owned := true
	defer func() {
		if owned {
			globalFree.Call(h)
		}
	}()
	if len(data) != 0 {
		p, _, lockErr := globalLock.Call(h)
		if p == 0 {
			return winError("GlobalLock", lockErr)
		}
		copy(unsafe.Slice((*byte)(nativePointer(p)), len(data)), data)
		globalUnlock.Call(h)
	}
	result, _, setErr := setClipboardData.Call(format, h)
	if result == 0 {
		return winError("SetClipboardData", setErr)
	}
	owned = false
	return nil
}

// Copy places text in the Windows clipboard, excluding it from history and cloud sync.
func Copy(text string) error {
	wide, err := windows.UTF16FromString(text)
	if err != nil {
		return err
	}
	formats := [3]uintptr{}
	for i, name := range [...]string{
		"ExcludeClipboardContentFromMonitorProcessing",
		"CanIncludeInClipboardHistory",
		"CanUploadToCloudClipboard",
	} {
		formats[i], err = registeredFormat(name)
		if err != nil {
			return err
		}
	}
	data := make([]byte, len(wide)*2)
	for i, c := range wide {
		binary.LittleEndian.PutUint16(data[i*2:], c)
	}
	return withClipboard(func() error {
		ok, _, callErr := emptyClipboard.Call()
		if ok == 0 {
			return winError("EmptyClipboard", callErr)
		}
		// A copy that could not be marked as excluded must not stay on the clipboard.
		fail := func(err error) error {
			emptyClipboard.Call()
			return err
		}
		if err := setBytes(cfUnicodeText, data); err != nil {
			return fail(err)
		}
		if err := setBytes(formats[0], nil); err != nil {
			return fail(err)
		}
		zero := make([]byte, 4)
		for _, format := range formats[1:] {
			if err := setBytes(format, zero); err != nil {
				return fail(err)
			}
		}
		return nil
	})
}

// ClearIfUnchanged empties the clipboard only when its Unicode text matches text.
func ClearIfUnchanged(text string) error {
	return withClipboard(func() error {
		available, _, _ := isFormatAvailable.Call(cfUnicodeText)
		if available == 0 {
			return nil
		}
		h, _, callErr := getClipboardData.Call(cfUnicodeText)
		if h == 0 {
			return winError("GetClipboardData", callErr)
		}
		p, _, lockErr := globalLock.Call(h)
		if p == 0 {
			return winError("GlobalLock", lockErr)
		}
		size, _, _ := globalSize.Call(h)
		words := unsafe.Slice((*uint16)(nativePointer(p)), size/2)
		end := 0
		for end < len(words) && words[end] != 0 {
			end++
		}
		current := string(utf16.Decode(words[:end]))
		globalUnlock.Call(h)
		if current != text {
			return nil
		}
		ok, _, emptyErr := emptyClipboard.Call()
		if ok == 0 {
			return winError("EmptyClipboard", emptyErr)
		}
		return nil
	})
}
