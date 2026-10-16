//go:build darwin

package main

/*
#cgo LDFLAGS: -framework CoreGraphics
#include <CoreGraphics/CoreGraphics.h>
*/
import "C"

import "github.com/sstallion/go-hid"

// hidSetup runs after hid.Init, which turns on exclusive (seize) mode.
// Seizing a keyboard needs root; writing LED reports does not need it.
func hidSetup() {
	hid.SetOpenExclusive(false)
}

// hostLockSymbol returns the host Caps Lock state as a symbol (bit 1);
// macOS has no other lock on this keyboard.
func hostLockSymbol() byte {
	flags := C.CGEventSourceFlagsState(C.kCGEventSourceStateCombinedSessionState)
	if uint64(flags)&uint64(C.kCGEventFlagMaskAlphaShift) != 0 {
		return 0x02
	}
	return 0
}
