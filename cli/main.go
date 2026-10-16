// kc - Keychron K10 Pro wireless remote control (protocol v2)
//
// Sends commands to the keyboard through HID LED output reports.
// Requires the bt_control firmware on the keyboard.
//
// macOS:   HID output reports (needs sudo or Input Monitoring permission)
// Windows: IOCTL_KEYBOARD_SET_INDICATORS on the keyboard \KBD endpoints
package main

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/sstallion/go-hid"
)

const (
	keychronVID = 0x3434
	stepDelay   = 80 * time.Millisecond
	abortDelay  = 350 * time.Millisecond // > LED_CMD_WINDOW_MS, lets the keyboard drop a broken frame

	symSeparator = 0
	symMagic     = 15
	symDigits    = 14 // parameter digits are 1..14, value range 0..195

	// Exit codes
	exitError = 1 // keyboard not found or the command could not be sent
	exitUsage = 2 // unknown command or bad argument
)

type command struct {
	code  byte
	label string
	param bool
}

// Command codes must match led_bt_control.h
var commands = map[string]command{
	"bt1":     {1, "Switch to BT 1", false},
	"bt2":     {2, "Switch to BT 2", false},
	"bt3":     {3, "Switch to BT 3", false},
	"bat":     {4, "Battery level", false},
	"off":     {5, "Disconnect BT", false},
	"rgb":     {6, "RGB toggle", false},
	"rgb_on":  {7, "RGB on", false},
	"rgb_off": {8, "RGB off", false},
	"mac":     {9, "Mac layout", false},
	"win":     {10, "Windows layout", false},
	"sleep":   {11, "Keyboard sleep", false},
	"mode":    {12, "RGB mode", true},
	"bright":  {13, "RGB brightness", true},
}

// link sends frame symbols to the keyboard; implemented per platform.
// restore resends the host's real lock state after a frame: QMK only
// reports changes, so a host report equal to the trailing 0 would be lost.
type link interface {
	send(sym byte) error
	restore() error
	close()
}

// buildFrame returns the symbols of one frame. Every symbol is followed by
// a separator so no two consecutive bytes are equal: QMK only reports LED
// state changes, so a repeated byte would be lost. value < 0 means none.
func buildFrame(code byte, value int) []byte {
	frame := []byte{symMagic, code, symSeparator}
	if value >= 0 {
		hi := byte(value/symDigits + 1)
		lo := byte(value%symDigits + 1)
		frame = append(frame, hi, symSeparator, lo, symSeparator)
	}
	return frame
}

// symbolToHID maps symbol bits 0-3 to HID LED bits 0 (Num), 1 (Caps),
// 2 (Scroll) and 4 (Kana).
func symbolToHID(sym byte) byte {
	return (sym & 0x07) | ((sym & 0x08) << 1)
}

type keyboard struct {
	path string
	name string
}

// findKeyboards returns every keyboard endpoint (usage page 1, usage 6).
func findKeyboards() ([]keyboard, error) {
	var found []keyboard
	err := hid.Enumerate(hid.VendorIDAny, hid.ProductIDAny, func(info *hid.DeviceInfo) error {
		if info.VendorID == keychronVID && info.UsagePage == 1 && info.Usage == 6 {
			found = append(found, keyboard{path: info.Path, name: info.ProductStr})
		}
		return nil
	})
	return found, err
}

func sendFrame(l link, frame []byte) error {
	last := len(frame) - 1
	for i, sym := range frame {
		// The trailing separator may fail if a BT switch already dropped the link
		if err := l.send(sym); err != nil && i != last {
			return err
		}
		time.Sleep(stepDelay)
	}
	return nil
}

func runFrame(kbs []keyboard, frame []byte) error {
	l, err := openLink(kbs)
	if err != nil {
		return err
	}
	defer l.close()

	err = sendFrame(l, frame)
	// A broken frame must time out first, or the lock state is read as a command
	if err != nil {
		time.Sleep(abortDelay)
	}
	_ = l.restore()
	return err
}

func usage() {
	fmt.Println(`kc - Keychron K10 Pro remote control

Bluetooth:
  kc bt1              Switch to BT device 1
  kc bt2              Switch to BT device 2
  kc bt3              Switch to BT device 3
  kc bat              Show battery level
  kc off              Disconnect BT

RGB:
  kc rgb              Toggle RGB
  kc rgb_on           Turn on RGB
  kc rgb_off          Turn off RGB
  kc mode <1-31>      Set RGB effect
  kc bright <1-31>    Set RGB brightness

Layout:
  kc mac              Mac layout
  kc win              Windows layout

Power:
  kc sleep            Keyboard sleep

Info:
  kc status           Connection info

Flags:
  -s, --silent        No output on success; errors still go to stderr

Exit codes: 0 success, 1 keyboard/send error, 2 bad command or argument`)
}

func printStatus(kbs []keyboard) {
	fmt.Printf("  Keyboard:  %s\n", kbs[0].name)
	fmt.Printf("  Method:    %s\n", linkMethod)
	for _, kb := range kbs {
		fmt.Printf("  Endpoint:  %s\n", kb.path)
	}
}

func main() {
	silent := false
	var args []string
	for _, a := range os.Args[1:] {
		if a == "-s" || a == "--silent" {
			silent = true
		} else {
			args = append(args, a)
		}
	}

	if len(args) == 0 || args[0] == "-h" || args[0] == "--help" || args[0] == "help" {
		usage()
		return
	}

	name := strings.ToLower(args[0])

	if err := hid.Init(); err != nil {
		fmt.Fprintf(os.Stderr, "Error: hid init: %v\n", err)
		os.Exit(exitError)
	}
	defer hid.Exit()

	kbs, err := findKeyboards()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: hid enumerate: %v\n", err)
		os.Exit(exitError)
	}

	if name == "status" {
		if len(kbs) == 0 {
			fmt.Fprintln(os.Stderr, "K10 Pro not found. Is BT connected?")
			os.Exit(exitError)
		}
		printStatus(kbs)
		return
	}

	cmd, ok := commands[name]
	if !ok {
		fmt.Fprintf(os.Stderr, "Unknown command: %s (kc --help for the list)\n", name)
		os.Exit(exitUsage)
	}

	value := -1
	if cmd.param {
		if len(args) < 2 {
			fmt.Fprintf(os.Stderr, "Usage: kc %s <1-31>\n", name)
			os.Exit(exitUsage)
		}
		v, err := strconv.Atoi(args[1])
		if err != nil || v < 1 || v > 31 {
			fmt.Fprintf(os.Stderr, "Value must be 1-31, got: %s\n", args[1])
			os.Exit(exitUsage)
		}
		value = v
	}

	if len(kbs) == 0 {
		fmt.Fprintln(os.Stderr, "K10 Pro not found. Is BT connected?")
		os.Exit(exitError)
	}

	if err := runFrame(kbs, buildFrame(cmd.code, value)); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		if permissionHint != "" {
			fmt.Fprintln(os.Stderr, permissionHint)
		}
		os.Exit(exitError)
	}

	if !silent {
		if value >= 0 {
			fmt.Printf("✓ %s → %d\n", cmd.label, value)
		} else {
			fmt.Printf("✓ %s\n", cmd.label)
		}
	}
}
