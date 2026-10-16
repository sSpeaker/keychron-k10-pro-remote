//go:build windows

package main

import (
	"encoding/binary"
	"fmt"
	"strings"
	"syscall"
)

const (
	linkMethod     = "IOCTL_KEYBOARD_SET_INDICATORS"
	permissionHint = ""

	// CTL_CODE(FILE_DEVICE_KEYBOARD, 0x0002, METHOD_BUFFERED, FILE_ANY_ACCESS)
	ioctlKeyboardSetIndicators = 0x000B0008

	// KEYBOARD_INDICATOR_PARAMETERS.LedFlags (ntddkbd.h)
	flagScroll = 0x01
	flagNum    = 0x02
	flagCaps   = 0x04
	flagKana   = 0x08

	vkCapital = 0x14
	vkNumLock = 0x90
	vkScroll  = 0x91
)

var procGetKeyState = syscall.NewLazyDLL("user32.dll").NewProc("GetKeyState")

func hidSetup() {}

type ioctlLink struct {
	handles []syscall.Handle
}

// openLink opens every keyboard endpoint through its \KBD path. Windows
// accepts the IOCTL on all of them, but only one forwards it to the
// keyboard, so each symbol is sent to all of them.
func openLink(kbs []keyboard) (link, error) {
	l := &ioctlLink{}
	var lastErr error
	for _, kb := range kbs {
		path := kb.path
		if !strings.HasSuffix(strings.ToUpper(path), `\KBD`) {
			path += `\KBD`
		}
		p, err := syscall.UTF16PtrFromString(path)
		if err != nil {
			lastErr = err
			continue
		}
		h, err := syscall.CreateFile(p, syscall.GENERIC_WRITE,
			syscall.FILE_SHARE_READ|syscall.FILE_SHARE_WRITE,
			nil, syscall.OPEN_EXISTING, 0, 0)
		if err != nil {
			lastErr = err
			continue
		}
		l.handles = append(l.handles, h)
	}
	if len(l.handles) == 0 {
		return nil, fmt.Errorf("open keyboard: %v", lastErr)
	}
	return l, nil
}

// symbolToFlags maps symbol bits 0-3 (HID Num/Caps/Scroll/Kana) to the
// LedFlags that the Windows HID stack translates back into those HID bits.
func symbolToFlags(sym byte) uint16 {
	var f uint16
	if sym&0x01 != 0 {
		f |= flagNum
	}
	if sym&0x02 != 0 {
		f |= flagCaps
	}
	if sym&0x04 != 0 {
		f |= flagScroll
	}
	if sym&0x08 != 0 {
		f |= flagKana
	}
	return f
}

func (l *ioctlLink) setFlags(flags uint16) error {
	var params [4]byte // KEYBOARD_INDICATOR_PARAMETERS: UnitId, LedFlags
	binary.LittleEndian.PutUint16(params[2:4], flags)

	var lastErr error
	ok := false
	for _, h := range l.handles {
		var returned uint32
		err := syscall.DeviceIoControl(h, ioctlKeyboardSetIndicators,
			&params[0], uint32(len(params)), nil, 0, &returned, nil)
		if err == nil {
			ok = true
		} else {
			lastErr = err
		}
	}
	if !ok {
		return fmt.Errorf("DeviceIoControl: %v", lastErr)
	}
	return nil
}

func (l *ioctlLink) send(sym byte) error {
	return l.setFlags(symbolToFlags(sym))
}

func toggled(vk uintptr) bool {
	r, _, _ := procGetKeyState.Call(vk)
	return r&1 != 0
}

// restore puts the real Windows lock state back on the keyboard.
func (l *ioctlLink) restore() error {
	var flags uint16
	if toggled(vkScroll) {
		flags |= flagScroll
	}
	if toggled(vkNumLock) {
		flags |= flagNum
	}
	if toggled(vkCapital) {
		flags |= flagCaps
	}
	return l.setFlags(flags)
}

func (l *ioctlLink) close() {
	for _, h := range l.handles {
		syscall.CloseHandle(h)
	}
	l.handles = nil
}
