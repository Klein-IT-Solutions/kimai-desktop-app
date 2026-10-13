//go:build windows

package hotkey

import (
	"context"
	"errors"
	"runtime"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"
)

// BackendWin32 registers the shortcut with RegisterHotKey.
const BackendWin32 = "win32"

var (
	user32                 = windows.NewLazySystemDLL("user32.dll")
	procRegisterHotKey     = user32.NewProc("RegisterHotKey")
	procUnregisterHotKey   = user32.NewProc("UnregisterHotKey")
	procGetMessageW        = user32.NewProc("GetMessageW")
	procPeekMessageW       = user32.NewProc("PeekMessageW")
	procPostThreadMessageW = user32.NewProc("PostThreadMessageW")
	procGetAsyncKeyState   = user32.NewProc("GetAsyncKeyState")
)

const (
	modAlt      = 0x0001
	modControl  = 0x0002
	modShift    = 0x0004
	modWin      = 0x0008
	modNoRepeat = 0x4000

	wmQuit   = 0x0012
	wmHotkey = 0x0312

	errHotkeyAlreadyRegistered = windows.Errno(1409)
)

type winMsg struct {
	hwnd    uintptr
	message uint32
	wParam  uintptr
	lParam  uintptr
	time    uint32
	pt      struct{ x, y int32 }
	private uint32
}

// win32 returns the RegisterHotKey modifiers and virtual-key code.
func (a Accelerator) win32() (mods, vk uint32) {
	if a.Super {
		mods |= modWin
	}
	if a.Ctrl {
		mods |= modControl
	}
	if a.Alt {
		mods |= modAlt
	}
	if a.Shift {
		mods |= modShift
	}
	switch {
	case fNumber(a.Key) > 0:
		vk = 0x70 + uint32(fNumber(a.Key)-1) // VK_F1…
	case a.Key == "space":
		vk = 0x20
	default:
		c := a.Key[0] // VK codes for letters are the upper-case ASCII codes, digits their own
		if c >= 'a' && c <= 'z' {
			c -= 'a' - 'A'
		}
		vk = uint32(c)
	}
	return mods, vk
}

func register(ctx context.Context, acc Accelerator, _ Shortcut, onPress, onRelease func(), _ func(Status)) Status {
	mods, vk := acc.win32()
	result := make(chan error, 1)
	go func() {
		// Hotkey messages go to the registering thread's queue.
		runtime.LockOSThread()
		defer runtime.UnlockOSThread()
		const id = 1
		if r, _, err := procRegisterHotKey.Call(0, id, uintptr(mods|modNoRepeat), uintptr(vk)); r == 0 {
			result <- err
			return
		}
		defer procUnregisterHotKey.Call(0, id)

		var m winMsg
		procPeekMessageW.Call(uintptr(unsafe.Pointer(&m)), 0, 0, 0, 0) // make sure the queue exists
		tid := windows.GetCurrentThreadId()
		stop := context.AfterFunc(ctx, func() { procPostThreadMessageW.Call(uintptr(tid), wmQuit, 0, 0) })
		defer stop()
		result <- nil

		for {
			r, _, _ := procGetMessageW.Call(uintptr(unsafe.Pointer(&m)), 0, 0, 0)
			if int32(r) <= 0 {
				return
			}
			if m.message == wmHotkey {
				onPress()
				waitKeyUp(ctx, vk)
				onRelease()
			}
		}
	}()

	if err := <-result; err != nil {
		msg := "Das Tastenkürzel konnte nicht registriert werden: " + err.Error()
		if errors.Is(err, errHotkeyAlreadyRegistered) {
			msg = "Das Tastenkürzel wird bereits von Windows oder einem anderen Programm verwendet. Bitte wähle ein anderes."
		}
		return Status{Shortcut: acc.String(), Message: msg}
	}
	return Status{Active: true, Backend: BackendWin32, Shortcut: acc.String()}
}

// waitKeyUp polls until the main key is released (RegisterHotKey only
// reports presses), which push-to-talk needs.
func waitKeyUp(ctx context.Context, vk uint32) {
	for ctx.Err() == nil {
		if s, _, _ := procGetAsyncKeyState.Call(uintptr(vk)); s&0x8000 == 0 {
			return
		}
		time.Sleep(25 * time.Millisecond)
	}
}
