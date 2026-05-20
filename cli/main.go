// kc - Keychron K10 Pro wireless remote control
//
// Control your K10 Pro over Bluetooth from macOS or Windows.
// Requires bt_control firmware on the keyboard.
//
// Build:
//
//	go mod tidy && go build -o kc .
//
// Usage:
//
//	sudo ./kc bt1             Switch to BT device 1
//	sudo ./kc rgb             Toggle RGB
//	sudo ./kc mode 5          Set RGB effect
//	sudo ./kc status          Show connection info
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
	magic    = 0x18
	reportID = 0x01
	delay    = 80 * time.Millisecond
	vid      = 0x3434 // Keychron VID
)

// Command codes (must match led_bt_control.h)
var commands = map[string]struct {
	code  byte
	label string
	param bool // true = needs a value argument
}{
	"bt1":     {0x01, "Switch to BT 1", false},
	"bt2":     {0x02, "Switch to BT 2", false},
	"bt3":     {0x03, "Switch to BT 3", false},
	"bat":     {0x04, "Battery level", false},
	"off":     {0x05, "Disconnect BT", false},
	"rgb":     {0x06, "RGB toggle", false},
	"rgb_on":  {0x07, "RGB on", false},
	"rgb_off": {0x08, "RGB off", false},
	"mac":     {0x09, "Mac layout", false},
	"win":     {0x0A, "Windows layout", false},
	"sleep":   {0x0B, "Keyboard sleep", false},
	"mode":    {0x0C, "RGB mode", true},
	"bright":  {0x0D, "RGB brightness", true},
}

// findKeyboard returns the HID path for the K10 Pro keyboard endpoint.
func findKeyboard() (path, name string, err error) {
	found := false
	hid.Enumerate(hid.VendorIDAny, hid.ProductIDAny, func(info *hid.DeviceInfo) error {
		if info.VendorID == vid && info.UsagePage == 1 && info.Usage == 6 {
			path = info.Path
			name = info.ProductStr
			found = true
		}
		return nil
	})
	if !found {
		return "", "", fmt.Errorf("K10 Pro not found. Is BT connected?")
	}
	return path, name, nil
}

// send writes a byte to the device, waits, and returns any error.
func send(dev *hid.Device, b byte) error {
	_, err := dev.Write([]byte{reportID, b})
	if err != nil {
		return err
	}
	time.Sleep(delay)
	return nil
}

// runCommand opens the keyboard, sends the protocol sequence, and closes.
func runCommand(path string, code byte, value *byte) error {
	dev, err := hid.OpenPath(path)
	if err != nil {
		return fmt.Errorf("open: %w", err)
	}
	defer dev.Close()

	// Step 1: magic
	if err := send(dev, magic); err != nil {
		return err
	}

	// Step 2: command
	if err := send(dev, code); err != nil {
		return err
	}

	// Step 3 (parameterized only): value
	if value != nil {
		if err := send(dev, *value); err != nil {
			return err
		}
	}

	// Final step: clear (may fail on BT switch — that's OK)
	_ = send(dev, 0x00)
	return nil
}

func usage() {
	fmt.Println("kc - Keychron K10 Pro remote control")
	fmt.Println()
	fmt.Println("Bluetooth:")
	fmt.Println("  kc bt1             Switch to BT device 1")
	fmt.Println("  kc bt2             Switch to BT device 2")
	fmt.Println("  kc bt3             Switch to BT device 3")
	fmt.Println("  kc bat             Show battery level")
	fmt.Println("  kc off             Disconnect BT")
	fmt.Println()
	fmt.Println("RGB:")
	fmt.Println("  kc rgb             Toggle on/off")
	fmt.Println("  kc rgb_on          Turn on")
	fmt.Println("  kc rgb_off         Turn off")
	fmt.Println("  kc mode <1-31>     Set effect")
	fmt.Println("  kc bright <1-31>   Set brightness")
	fmt.Println()
	fmt.Println("Layout:")
	fmt.Println("  kc mac             Mac layout")
	fmt.Println("  kc win             Windows layout")
	fmt.Println()
	fmt.Println("Power:")
	fmt.Println("  kc sleep           Keyboard sleep")
	fmt.Println()
	fmt.Println("Info:")
	fmt.Println("  kc status          Connection info")
	fmt.Println()
	fmt.Println("Flags:")
	fmt.Println("  -s, --silent       No output on success")
}

func main() {
	// Parse flags
	silent := false
	args := []string{}
	for _, a := range os.Args[1:] {
		if a == "--silent" || a == "-s" {
			silent = true
		} else {
			args = append(args, a)
		}
	}

	if len(args) < 1 {
		usage()
		os.Exit(0)
	}

	arg := strings.ToLower(args[0])

	if arg == "-h" || arg == "--help" || arg == "help" {
		usage()
		os.Exit(0)
	}

	hid.Init()
	defer hid.Exit()

	if arg == "status" {
		path, name, err := findKeyboard()
		if err != nil {
			fmt.Println("  " + err.Error())
			os.Exit(1)
		}
		fmt.Printf("  Keyboard:  %s\n", name)
		fmt.Printf("  Path:      %s\n", path)
		fmt.Printf("  Transport: BLE HID\n")
		os.Exit(0)
	}

	cmd, ok := commands[arg]
	if !ok {
		fmt.Printf("Unknown command: %s\n\n", arg)
		usage()
		os.Exit(1)
	}

	var value *byte
	if cmd.param {
		if len(args) < 2 {
			fmt.Printf("Usage: kc %s <1-31>\n", arg)
			os.Exit(1)
		}
		v, err := strconv.Atoi(args[1])
		if err != nil || v < 1 || v > 31 {
			fmt.Printf("Value must be 1-31, got: %s\n", args[1])
			os.Exit(1)
		}
		b := byte(v)
		value = &b
	}

	path, _, err := findKeyboard()
	if err != nil {
		fmt.Println(err)
		os.Exit(1)
	}

	if err := runCommand(path, cmd.code, value); err != nil {
		errMsg := strings.ToLower(err.Error())
		if strings.Contains(errMsg, "privilege") || strings.Contains(errMsg, "permission") {
			fmt.Println("Error: HID access requires elevated privileges.")
			fmt.Printf("\n  sudo kc %s\n\n", strings.Join(args, " "))
			fmt.Println("  Or add Terminal to:")
			fmt.Println("  System Settings → Privacy & Security → Input Monitoring")
			os.Exit(1)
		}
		fmt.Printf("Error: %v\n", err)
		os.Exit(1)
	}

	if !silent {
		if value != nil {
			fmt.Printf("✓ %s → %d\n", cmd.label, *value)
		} else {
			fmt.Printf("✓ %s\n", cmd.label)
		}
	}
}
