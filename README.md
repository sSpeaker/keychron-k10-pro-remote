# kc — Keychron K10 Pro Wireless Remote Control

Control a Keychron K10 Pro over Bluetooth from macOS or Windows: switch BT profiles, toggle RGB, set effect and brightness, switch Mac/Windows layout, put the keyboard to sleep. No cable, no extra hardware.

## How It Works

A keyboard cannot receive arbitrary data from the host over Bluetooth HID, but it does receive the LED indicator state (Num Lock, Caps Lock, Scroll Lock, Compose, Kana). The host tool writes a sequence of LED states, and a modified QMK firmware decodes that sequence as a command.

```
Host (kc)                                  K10 Pro (QMK firmware)
  │ LED state 15  (magic)  ────────────►   start frame
  │ LED state 6   (command) ───────────►   queue "RGB toggle"
  │ LED state 0   (separator) ─────────►   frame closed
  │ real lock state (Windows) ─────────►   normal LED report
  │                                        300 ms later: rgb_matrix_toggle()
```

The host never changes its own lock state: Caps Lock, Num Lock and Scroll Lock keep working normally.

## Commands

| Command            | Action                         |
|--------------------|--------------------------------|
| `kc bt1`           | Switch to BT device 1          |
| `kc bt2`           | Switch to BT device 2          |
| `kc bt3`           | Switch to BT device 3          |
| `kc bat`           | Show battery level on LEDs     |
| `kc off`           | Disconnect BT                  |
| `kc rgb`           | Toggle RGB                     |
| `kc rgb_on`        | Turn on RGB                    |
| `kc rgb_off`       | Turn off RGB                   |
| `kc mode <1-31>`   | Set RGB effect                 |
| `kc bright <1-31>` | Set RGB brightness (31 = max)  |
| `kc mac`           | Mac layout (layer 0)           |
| `kc win`           | Windows layout (layer 2)       |
| `kc sleep`         | Put keyboard to sleep          |
| `kc status`        | Show connection info           |

`-s` / `--silent` (any position) prints nothing on success. Errors always go to stderr, so scripts can rely on the exit code:

| Exit code | Meaning |
|-----------|---------|
| 0 | command sent |
| 1 | keyboard not found or the command could not be sent |
| 2 | unknown command or bad argument |

```bash
kc -s bt2 || echo "switch failed" >&2
```

## Project Structure

```
├── bin/                     prebuilt CLI binaries (macOS arm64, Windows x64) and firmware
├── cli/                     Go CLI (single binary)
│   ├── main.go              commands, frame encoding, device discovery
│   ├── link_default.go      macOS: HID output reports via go-hid
│   ├── link_windows.go      Windows: IOCTL_KEYBOARD_SET_INDICATORS
│   ├── lock_darwin.go       macOS: real Caps Lock state via CoreGraphics
│   ├── lock_other.go        other Unix: no lock state available
│   ├── go.mod, go.sum
│   └── Makefile
├── firmware/                QMK keymap for K10 Pro ANSI RGB
│   ├── led_bt_control.h     protocol constants
│   ├── led_bt_control.c     frame parser, command execution, lock indicators
│   ├── keymap.c             default keymap + led_update_user / matrix_scan_user hooks
│   └── rules.mk
├── kc.py                    Python CLI (same protocol, needs hidapi)
├── .gitignore
└── README.md
```

## Protocol v2

### Why only 4 bits

| HID LED bit | Lock        | macOS | Windows |
|-------------|-------------|-------|---------|
| 0           | Num Lock    | yes   | yes     |
| 1           | Caps Lock   | yes   | yes     |
| 2           | Scroll Lock | yes   | yes     |
| 3           | Compose     | yes   | **no**  |
| 4           | Kana        | yes   | yes     |

Windows sets keyboard LEDs only through `IOCTL_KEYBOARD_SET_INDICATORS`, whose flags (`ntddkbd.h`) cover Scroll, Num, Caps and Kana. There is no Compose flag, so the protocol uses 4-bit symbols carried on HID bits 0, 1, 2 and 4.

| Symbol bit | HID bit       | Windows LedFlags      |
|------------|---------------|-----------------------|
| 0          | 0 (Num)       | `0x02` Num Lock       |
| 1          | 1 (Caps)      | `0x04` Caps Lock      |
| 2          | 2 (Scroll)    | `0x01` Scroll Lock    |
| 3          | 4 (Kana)      | `0x08` Kana           |

### Frames

QMK's `led_task()` only calls `led_update_user()` when the LED state changes, so two equal bytes in a row are lost. Every symbol is therefore followed by a separator `0`:

```
simple command:   15, cmd, 0
with parameter:   15, cmd, 0, hi, 0, lo, 0
```

- `15` — magic (HID `0x17` = Num + Caps + Scroll + Kana), starts a frame. Kana is never set on non-Japanese layouts, so normal lock states never produce it.
- `cmd` — command code 1–14.
- `hi`, `lo` — parameter digits 1–14, value = `(hi − 1) × 14 + (lo − 1)`, range 0–195.
- A gap over 300 ms, or a byte with the Compose bit, aborts the frame.

| Code | Command        | Parameter |
|------|----------------|-----------|
| 1    | BT device 1    | –         |
| 2    | BT device 2    | –         |
| 3    | BT device 3    | –         |
| 4    | Battery level  | –         |
| 5    | BT disconnect  | –         |
| 6    | RGB toggle     | –         |
| 7    | RGB on         | –         |
| 8    | RGB off        | –         |
| 9    | Mac layer      | –         |
| 10   | Windows layer  | –         |
| 11   | Sleep          | –         |
| 12   | RGB mode       | effect number |
| 13   | RGB brightness | 1–31      |
| 14   | free           |           |

Each step takes 80 ms: a simple command takes about 240 ms, a command with a parameter about 560 ms.

## Install — Go CLI

Single binary with no runtime dependencies. Building needs Go 1.21+ and a C compiler (CGo), because `go-hid` compiles hidapi from source.

**macOS**

```bash
cd cli
make                  # go mod tidy + go build
sudo make install     # /usr/local/bin/kc
sudo kc bt2
```

To run without `sudo`: System Settings → Privacy & Security → Input Monitoring → add the terminal app.

**Windows (PowerShell)**

Install Go (`winget install -e --id GoLang.Go`) and GCC from [WinLibs](https://github.com/brechtsanders/winlibs_mingw/releases) (extract to `C:\mingw64`).

```powershell
$env:Path += ";C:\mingw64\mingw64\bin"
cd cli
go mod tidy
go build -o kc.exe .
.\kc.exe bt2
```

Or cross-compile from macOS with `brew install mingw-w64` and `make windows`; `make release` rebuilds both prebuilt binaries in `bin/`.

No administrator rights are needed. The tool opens the keyboard's `\KBD` endpoints, sends each symbol to all of them (Windows accepts the IOCTL on every endpoint but forwards it from one), and afterwards restores the real Num/Caps/Scroll state read with `GetKeyState`.

## Install — Python CLI

```bash
# macOS
brew install hidapi && pip install hid
sudo python3 kc.py bt2

# Windows
pip install hidapi
python kc.py bt2
```

## Install — Firmware

### Prerequisites

- Keychron QMK fork, branch `bluetooth_playground` (the K10 Pro is not in mainline QMK)
- `arm-none-eabi-gcc` with newlib. Homebrew's `arm-none-eabi-gcc` 16.x ships without newlib (`stdint.h: No such file or directory`); use the xPack build instead.
- QMK Toolbox or `dfu-util` for flashing

### Build

```bash
git clone --recurse-submodules --branch bluetooth_playground \
  https://github.com/Keychron/qmk_firmware.git keychron-qmk
cd keychron-qmk
pip install -r requirements.txt

curl -LO https://github.com/xpack-dev-tools/arm-none-eabi-gcc-xpack/releases/download/v13.3.1-1.1/xpack-arm-none-eabi-gcc-13.3.1-1.1-darwin-arm64.tar.gz
tar xf xpack-arm-none-eabi-gcc-13.3.1-1.1-darwin-arm64.tar.gz
export PATH="$PWD/xpack-arm-none-eabi-gcc-13.3.1-1.1/bin:$PATH"

mkdir -p keyboards/keychron/k10_pro/ansi/rgb/keymaps/bt_control
cp /path/to/this/repo/firmware/* keyboards/keychron/k10_pro/ansi/rgb/keymaps/bt_control/

make keychron/k10_pro/ansi/rgb:bt_control
```

Output: `keychron_k10_pro_ansi_rgb_bt_control.bin`

### Flash

1. Switch on the back → **Off**, unplug USB
2. Hold **Esc**, plug in USB, switch → **Cable**
3. Keep holding Esc 2–3 s, release
4. QMK Toolbox → Open the `.bin` → **Flash** (or `make keychron/k10_pro/ansi/rgb:bt_control:flash`)
5. Switch → **BT**

If DFU mode does not start, remove the spacebar keycap and hold the Reset button under it while plugging in USB.

The firmware and the host tools must be the same protocol version: v1 tools (magic `0x18`) do not work with v2 firmware.

## Implementation Notes

**Deferred execution.** Frames are parsed in `led_update_user()`, but commands run 300 ms later from `matrix_scan_user()`. QMK functions such as `rgb_matrix_toggle()` only behave correctly from the main loop, and for a BT switch the delay lets the host finish the frame and restore its lock state before the connection drops.

**Lock indicators.** The magic symbol includes Caps and Num, so drawing the indicators from the raw LED state would flash them on every command. The firmware remembers the last LED state received outside a frame and overrides Keychron's weak `os_state_indicate()` to draw from it.

**Lock state resend.** A frame ends with LED state `0`, and QMK only reports changes. If the host's real state is not `0` (Caps Lock on), its next report of `0` would be dropped and the indicator would stay lit. After every frame the host therefore resends its real lock state: Windows reads it with `GetKeyState`, macOS reads Caps Lock with `CGEventSourceFlagsState`. If a frame breaks halfway, the tool waits 350 ms first so the keyboard times the frame out; otherwise the resent state could be read as a command byte.

**Layout.** `mac` / `win` change the default layer, exactly like the physical Mac/Win switch. Flipping the switch overrides the command.

**Wake-up tap.** After a BT profile switch the firmware sends a phantom F24 keypress after 500 ms to activate the new connection. F24 is ignored by modern operating systems.

**Sleep.** Uses `LOW_POWER_MODE` (`PM_STOP1`), the same mode as Keychron's auto-sleep. `rgb_matrix_driver_shutdown()` turns the LED driver off first; it comes back on the first flush after wakeup. Any key wakes the keyboard.

**RGB.** `rgb_on` / `rgb_off` use `rgb_matrix_toggle()` with a state check. `mode` and `bright` enable RGB first, because `rgb_matrix_mode()` is ignored while RGB is disabled.

## Compatibility

- Keyboard: Keychron K10 Pro, ANSI, RGB
- Host: macOS (BLE HID), Windows (Bluetooth Classic HID)
- QMK: Keychron fork, `bluetooth_playground`

## License

GPL-2.0-or-later (same as QMK)
