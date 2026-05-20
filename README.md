# kc — Keychron K10 Pro Wireless Remote Control

Control your Keychron K10 Pro over Bluetooth — switch BT profiles, toggle RGB, change OS layout, manage sleep — all from the command line, no USB cable needed.

## How It Works

BLE HID keyboards receive LED indicator reports from the host (Num Lock, Caps Lock, Scroll Lock, Compose, Kana — 5 bits). Compose and Kana are unused on modern systems.

This project uses all 5 bits as a command channel. A modified QMK firmware on the keyboard recognizes a magic byte (`0x18`) and interprets the next byte as a command.

```
Host (macOS/Windows)                Keyboard (QMK firmware)
    │                                    │
    ├── LED byte: 0x18 (magic) ────────► ARMED
    │       80ms
    ├── LED byte: 0x06 (command) ──────► queue: rgb_toggle
    │       80ms
    ├── LED byte: 0x00 (clear) ────────► IDLE
    │                                    │
    │         200ms deferred             │
    │                                    ├── rgb_matrix_toggle()
```

Commands are deferred 200ms so the host finishes sending all bytes before BT disconnect (for profile switching) and to run in the correct QMK execution context (`matrix_scan` instead of `led_update`).

## Commands

| Command           | Action                        |
|-------------------|-------------------------------|
| `kc bt1`          | Switch to BT device 1         |
| `kc bt2`          | Switch to BT device 2         |
| `kc bt3`          | Switch to BT device 3         |
| `kc bat`          | Show battery level on LEDs    |
| `kc off`          | Disconnect BT                 |
| `kc rgb`          | Toggle RGB on/off             |
| `kc rgb_on`       | Turn on RGB                   |
| `kc rgb_off`      | Turn off RGB                  |
| `kc mode <1-31>`  | Set RGB effect by number      |
| `kc bright <1-31>`| Set RGB brightness (31 = max) |
| `kc mac`          | Switch to Mac layout          |
| `kc win`          | Switch to Windows layout      |
| `kc sleep`        | Put keyboard to deep sleep    |
| `kc status`       | Show connection info          |

All commands support `-s` / `--silent` flag to suppress output on success.

## Project Structure

```
├── cli/                        Go CLI (single binary, no dependencies)
│   ├── main.go
│   ├── go.mod
│   └── Makefile
├── firmware/                   QMK keymap for K10 Pro ANSI RGB
│   ├── led_bt_control.h        Protocol constants
│   ├── led_bt_control.c        State machine + command execution
│   ├── keymap.c                Default keymap + hooks
│   └── rules.mk                Build config
├── kc.py                       Python CLI (alternative, needs hidapi)
├── .gitignore
└── README.md
```

## Install — Go CLI (recommended)

Single binary, no runtime dependencies. Works on macOS and Windows.

**Prerequisites:** Go 1.21+ with CGo support (GCC must be in PATH).

```bash
cd cli
make
sudo make install    # installs to /usr/local/bin/kc
```

Or manually:

```bash
cd cli
go get github.com/sstallion/go-hid@latest
go build -o kc .
sudo cp kc /usr/local/bin/
```

**Usage:**

```bash
sudo kc bt1
sudo kc rgb
sudo kc mode 5
sudo kc bright 20 --silent
```

To avoid `sudo` on macOS: System Settings → Privacy & Security → Input Monitoring → add your Terminal app.

## Install — Python CLI (alternative)

Lighter setup if Go is not available.

**Prerequisites:**

```bash
brew install hidapi     # macOS
pip install hid         # Python bindings
```

**Usage:**

```bash
sudo python3 kc.py bt1
sudo python3 kc.py rgb
sudo python3 kc.py mode 5
```

## Install — Firmware

### Prerequisites

- Keychron QMK fork, `bluetooth_playground` branch
- ARM cross-compiler (`arm-none-eabi-gcc` 13.x with newlib)
- QMK Toolbox for flashing (macOS/Windows)

### Build

```bash
# Clone Keychron's QMK fork
git clone --recurse-submodules --branch bluetooth_playground \
  https://github.com/Keychron/qmk_firmware.git
cd qmk_firmware

# Install ARM toolchain
# macOS:
brew install arm-none-eabi-gcc
# If Homebrew version lacks newlib, use xPack:
curl -LO https://github.com/xpack-dev-tools/arm-none-eabi-gcc-xpack/releases/download/v13.3.1-1.1/xpack-arm-none-eabi-gcc-13.3.1-1.1-darwin-arm64.tar.gz
tar xf xpack-arm-none-eabi-gcc-13.3.1-1.1-darwin-arm64.tar.gz
export PATH="$PWD/xpack-arm-none-eabi-gcc-13.3.1-1.1/bin:$PATH"

# Install QMK Python dependencies
pip install -r requirements.txt

# Copy firmware into keymap directory
mkdir -p keyboards/keychron/k10_pro/ansi/rgb/keymaps/bt_control
cp /path/to/this/repo/firmware/* \
  keyboards/keychron/k10_pro/ansi/rgb/keymaps/bt_control/

# Build
make keychron/k10_pro/ansi/rgb:bt_control
```

Output: `keychron_k10_pro_ansi_rgb_bt_control.bin`

### Flash

1. Disconnect USB cable
2. Toggle switch → **Off**
3. Hold **Esc** key
4. Plug in USB cable, toggle → **Cable**
5. Hold Esc 3 more seconds, then release
6. QMK Toolbox → Open `.bin` → Flash
7. After flashing, toggle → **BT**

If DFU mode doesn't activate with Esc, use the hardware reset button (pinhole on the bottom under the spacebar).

## Protocol Reference

5-bit encoding over HID LED output report. Report ID `0x01` on macOS BLE.

| Code | Command        | Type   |
|------|----------------|--------|
| 0x01 | BT device 1    | direct |
| 0x02 | BT device 2    | direct |
| 0x03 | BT device 3    | direct |
| 0x04 | Battery level  | direct |
| 0x05 | BT disconnect  | direct |
| 0x06 | RGB toggle     | direct |
| 0x07 | RGB on         | direct |
| 0x08 | RGB off        | direct |
| 0x09 | Mac layer      | direct |
| 0x0A | Win layer      | direct |
| 0x0B | Sleep          | direct |
| 0x0C | Set RGB mode   | param  |
| 0x0D | Set brightness | param  |

**Direct commands** — 3 steps: `magic → command → clear`

**Parameterized commands** — 4 steps: `magic → command → value (1-31) → clear`

Codes 0x0E–0x1F are free for future commands.

## Implementation Notes

**Deferred execution.** All commands are parsed in `led_update_user()` (LED interrupt context) but executed in `led_bt_control_task()` via `matrix_scan_user()` (main loop context). This is required because QMK functions like `rgb_matrix_toggle()` only work from the main loop.

**BT switch delay.** BT profile commands are deferred 200ms so the host finishes the full protocol before the keyboard disconnects from the current BLE connection.

**Wake-up tap.** After switching BT profiles, the firmware sends a phantom F24 keypress (500ms delay) to activate the new BLE connection. F24 is ignored by all modern OSes.

**Sleep mode.** Uses `PM_STOP1` (STM32L432 Stop mode 1) — same as Keychron's built-in auto-sleep. Microamp power consumption, instant wakeup on any keypress. `rgb_matrix_driver_shutdown()` physically turns off the LED driver chip before sleep.

**RGB control.** `rgb_on`/`rgb_off` use `rgb_matrix_toggle()` with state check (idempotent). `mode` and `bright` auto-enable RGB if it was off. All changes persist in EEPROM across power cycles (but not across reflashes).

## Compatibility

- **Keyboard:** Keychron K10 Pro, ANSI layout, RGB
- **Host OS:** macOS (tested on macOS 26 / Apple Silicon), Windows (untested, should work)
- **QMK branch:** `bluetooth_playground` (Keychron fork)
- **BT chip:** CKBT51 (Broadcom) — closed firmware, not modified

## License

GPL-2.0-or-later (same as QMK)
