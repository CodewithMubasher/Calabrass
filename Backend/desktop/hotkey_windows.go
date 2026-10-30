//go:build windows

package main

import (
	"log/slog"
	"runtime"
	"syscall"
	"unsafe"
)

// The global hotkey is Ctrl+Alt+Space: it brings Calabrass to the front from any
// program. It is registered on a thread of its own, because Windows delivers a
// hotkey to the thread that registered it, and that thread must run a message
// loop. Any failure (the combination is taken by another program, say) is logged
// and the app carries on without it.

const (
	modAlt      = 0x0001
	modControl  = 0x0002
	modNoRepeat = 0x4000
	vkSpace     = 0x20
	wmHotkey    = 0x0312
	wmQuit      = 0x0012
	hotkeyID    = 0xCA1A
)

var (
	user32            = syscall.NewLazyDLL("user32.dll")
	kernel32          = syscall.NewLazyDLL("kernel32.dll")
	procRegisterHot   = user32.NewProc("RegisterHotKey")
	procUnregisterHot = user32.NewProc("UnregisterHotKey")
	procGetMessage    = user32.NewProc("GetMessageW")
	procPostThreadMsg = user32.NewProc("PostThreadMessageW")
	procThreadID      = kernel32.NewProc("GetCurrentThreadId")
)

type point struct{ x, y int32 }

type msg struct {
	hwnd    uintptr
	message uint32
	wParam  uintptr
	lParam  uintptr
	time    uint32
	pt      point
}

func registerShowHotkey(log *slog.Logger, onPress func()) (stop func()) {
	threadID := make(chan uint32, 1)
	done := make(chan struct{})

	go func() {
		defer close(done)
		defer func() {
			if r := recover(); r != nil {
				log.Warn("the global hotkey stopped", "error", r)
			}
		}()
		runtime.LockOSThread()
		defer runtime.UnlockOSThread()

		id, _, _ := procThreadID.Call()
		ok, _, err := procRegisterHot.Call(0, hotkeyID, modControl|modAlt|modNoRepeat, vkSpace)
		if ok == 0 {
			log.Info("the global hotkey Ctrl+Alt+Space is unavailable", "reason", err)
			threadID <- 0
			return
		}
		defer procUnregisterHot.Call(0, hotkeyID)
		threadID <- uint32(id)

		var m msg
		for {
			r, _, _ := procGetMessage.Call(uintptr(unsafe.Pointer(&m)), 0, 0, 0)
			// 0 is WM_QUIT, -1 is an error: either way the loop is over.
			if int32(r) <= 0 {
				return
			}
			if m.message == wmHotkey && m.wParam == hotkeyID {
				onPress()
			}
		}
	}()

	id := <-threadID
	return func() {
		if id != 0 {
			procPostThreadMsg.Call(uintptr(id), wmQuit, 0, 0)
			<-done
		}
	}
}
