//go:build windows && (amd64 || arm64)

package wininput

import (
	"fmt"
	"sync"
	"unicode/utf16"
	"unsafe"

	"golang.org/x/sys/windows"
)

// The Windows keyboard structures. x/sys does not declare these, and their
// layout is not ours to choose: SendInput reads cbSize bytes for every event,
// and the union inside INPUT is as wide as its widest member — the mouse one,
// even though keys are all this package ever sends. The widths are pinned by a
// test, because a wrong one is a memory error rather than a wrong keystroke.
type keybdInput struct {
	Code  uint16
	Scan  uint16
	Flags uint32
	Time  uint32
	Extra uintptr
}

type input struct {
	Kind uint32
	_    uint32 // the ABI aligns the union to eight bytes on 64-bit Windows
	Ki   keybdInput
	_    [8]byte // the rest of the union: MOUSEINPUT is 32 bytes, KEYBDINPUT 24
}

// INPUT_KEYBOARD, and the flags a keyboard event may carry.
const (
	kindKeyboard = 1
	flagKeyUp    = 0x0002
	// flagUnicode says that the event carries a character in wScan rather than a
	// virtual key, which is how text goes in without caring what keyboard layout
	// the machine is set to.
	flagUnicode = 0x0004
)

var sendInput = windows.NewLazySystemDLL("user32.dll").NewProc("SendInput")

// oneAtATime keeps two sequences from interleaving into nonsense. Typing is the
// only thing this package does, and it is always one thing at a time.
var oneAtATime sync.Mutex

// Type writes text as if it were typed.
func Type(text string) error {
	oneAtATime.Lock()
	defer oneAtATime.Unlock()
	return send(textEvents(text))
}

// Tap presses and releases one key.
func Tap(code uint16) error {
	oneAtATime.Lock()
	defer oneAtATime.Unlock()

	return send([]keybdInput{
		{Code: code},
		{Code: code, Flags: flagKeyUp},
	})
}

// send hands the events to Windows and reports what Windows made of them. A
// short count is the only real error here: Windows drops injected input when the
// window in front belongs to a higher privilege, and the count is how it says so.
func send(events []keybdInput) error {
	if len(events) == 0 {
		return nil
	}

	inputs := make([]input, 0, len(events))
	for _, event := range events {
		inputs = append(inputs, input{Kind: kindKeyboard, Ki: event})
	}

	sent, _, _ := sendInput.Call(
		uintptr(unsafe.Pointer(&inputs[0])),
		uintptr(len(inputs)),
		unsafe.Sizeof(input{}),
	)
	if sent != uintptr(len(inputs)) {
		return fmt.Errorf("windows accepted %d of the %d keystrokes", sent, len(inputs))
	}
	return nil
}

// textEvents turns text into the events that type it: a press and a release for
// every UTF-16 code unit, which is the unit Windows sends characters in.
// Anything outside the basic plane — an emoji, say — is two units, and typing it
// still works.
func textEvents(text string) []keybdInput {
	units := utf16.Encode([]rune(text))

	events := make([]keybdInput, 0, len(units)*2)
	for _, unit := range units {
		events = append(events,
			keybdInput{Scan: unit, Flags: flagUnicode},
			keybdInput{Scan: unit, Flags: flagUnicode | flagKeyUp},
		)
	}
	return events
}
