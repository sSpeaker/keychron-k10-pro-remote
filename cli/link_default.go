//go:build !windows

package main

import "github.com/sstallion/go-hid"

const (
	linkMethod     = "HID output report"
	reportID       = 0x01 // keyboard report ID on macOS BLE HID
	permissionHint = "  Run with sudo, or add the terminal app to\n" +
		"  System Settings → Privacy & Security → Input Monitoring"
)

type hidLink struct {
	dev *hid.Device
}

// openLink opens the first keyboard endpoint for the whole frame.
func openLink(kbs []keyboard) (link, error) {
	dev, err := hid.OpenPath(kbs[0].path)
	if err != nil {
		return nil, err
	}
	return &hidLink{dev: dev}, nil
}

func (l *hidLink) send(sym byte) error {
	_, err := l.dev.Write([]byte{reportID, symbolToHID(sym)})
	return err
}

func (l *hidLink) close() {
	l.dev.Close()
}
