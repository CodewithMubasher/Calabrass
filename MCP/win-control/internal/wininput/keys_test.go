//go:build windows && (amd64 || arm64)

package wininput

import (
	"testing"
	"unsafe"
)

// TestTheInputLayoutIsTheOneWindowsReads: SendInput reads one fixed-size struct
// per event, so a size or an offset that is off by a word is a memory error
// rather than a keystroke that lands in the wrong place.
func TestTheInputLayoutIsTheOneWindowsReads(t *testing.T) {
	if size := unsafe.Sizeof(input{}); size != 40 {
		t.Fatalf("input is %d bytes, want 40", size)
	}
	if offset := unsafe.Offsetof(input{}.Ki); offset != 8 {
		t.Errorf("the keyboard event starts at %d, want 8", offset)
	}
	// On a nested field Offsetof is measured from the inner struct, so the two
	// are added to get the offset from the start of the event.
	if offset := unsafe.Offsetof(input{}.Ki) + unsafe.Offsetof(keybdInput{}.Flags); offset != 12 {
		t.Errorf("the flags are at %d, want 12", offset)
	}
}

func TestTextIsAPressAndAReleaseForEveryCodeUnit(t *testing.T) {
	events := textEvents("ab")
	if len(events) != 4 {
		t.Fatalf("got %d events for two characters, want 4", len(events))
	}

	if events[0].Scan != 'a' || events[0].Flags != flagUnicode {
		t.Errorf("the first event is %+v, want a press of a", events[0])
	}
	if events[1].Scan != 'a' || events[1].Flags != flagUnicode|flagKeyUp {
		t.Errorf("the second event is %+v, want a release of a", events[1])
	}
	if events[2].Scan != 'b' {
		t.Errorf("the third event is %+v, want b", events[2])
	}
}

// TestAnEmojiIsTwoUnitsAndStillTyped: text outside the basic plane is not one
// code unit, and typing it has to work anyway.
func TestAnEmojiIsTwoUnitsAndStillTyped(t *testing.T) {
	if events := textEvents("🎵"); len(events) != 4 {
		t.Fatalf("got %d events for one emoji, want 4: it is two UTF-16 units", len(events))
	}
}

func TestNothingToTypeIsNoEvents(t *testing.T) {
	if events := textEvents(""); len(events) != 0 {
		t.Errorf("got %d events for empty text, want none", len(events))
	}
}
