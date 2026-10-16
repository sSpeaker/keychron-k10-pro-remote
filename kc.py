#!/usr/bin/env python3
"""
kc - Keychron K10 Pro remote control (protocol v2)

Dependencies:
    macOS:   brew install hidapi && pip install hid
    Windows: pip install hidapi

Usage:
    kc.py bt1              Switch to BT device 1
    kc.py bt2              Switch to BT device 2
    kc.py bt3              Switch to BT device 3
    kc.py bat              Show battery level
    kc.py off              Disconnect BT
    kc.py rgb              Toggle RGB
    kc.py rgb_on           Turn on RGB
    kc.py rgb_off          Turn off RGB
    kc.py mac              Mac layout
    kc.py win              Windows layout
    kc.py sleep            Keyboard sleep
    kc.py mode <1-31>      Set RGB effect
    kc.py bright <1-31>    Set RGB brightness
    kc.py status           Connection info

Flags:
    -s, --silent           No output on success; errors still go to stderr

Exit codes: 0 success, 1 keyboard/send error, 2 bad command or argument

macOS needs sudo or Input Monitoring permission for the terminal.
"""

import platform
import sys
import time

IS_WINDOWS   = platform.system() == 'Windows'
KEYCHRON_VID = 0x3434
REPORT_ID    = 0x01
STEP_DELAY   = 0.08

SYM_SEPARATOR = 0
SYM_MAGIC     = 15
SYM_DIGITS    = 14   # parameter digits are 1..14, value range 0..195

EXIT_ERROR = 1  # keyboard not found or the command could not be sent
EXIT_USAGE = 2  # unknown command or bad argument

# name: (command code, label, takes parameter) — must match led_bt_control.h
COMMANDS = {
    'bt1':     (1,  'Switch to BT 1',  False),
    'bt2':     (2,  'Switch to BT 2',  False),
    'bt3':     (3,  'Switch to BT 3',  False),
    'bat':     (4,  'Battery level',   False),
    'off':     (5,  'Disconnect BT',   False),
    'rgb':     (6,  'RGB toggle',      False),
    'rgb_on':  (7,  'RGB on',          False),
    'rgb_off': (8,  'RGB off',         False),
    'mac':     (9,  'Mac layout',      False),
    'win':     (10, 'Windows layout',  False),
    'sleep':   (11, 'Keyboard sleep',  False),
    'mode':    (12, 'RGB mode',        True),
    'bright':  (13, 'RGB brightness',  True),
}


# ── Frame encoding ───────────────────────────────────────────────────

def build_frame(code, value=None):
    """Symbols of one frame. Every symbol is followed by a separator so
    no two consecutive bytes are equal (QMK drops repeated LED states)."""
    frame = [SYM_MAGIC, code, SYM_SEPARATOR]
    if value is not None:
        hi = value // SYM_DIGITS + 1
        lo = value % SYM_DIGITS + 1
        frame += [hi, SYM_SEPARATOR, lo, SYM_SEPARATOR]
    return frame


def symbol_to_hid(sym):
    """Symbol bits 0-3 -> HID LED bits 0 (Num), 1 (Caps), 2 (Scroll), 4 (Kana)."""
    return (sym & 0x07) | ((sym & 0x08) << 1)


def symbol_to_win_flags(sym):
    """Symbol -> KEYBOARD_INDICATOR_PARAMETERS.LedFlags (ntddkbd.h):
    Scroll=0x01, Num=0x02, Caps=0x04, Kana=0x08."""
    flags = 0
    if sym & 0x01: flags |= 0x02  # Num
    if sym & 0x02: flags |= 0x04  # Caps
    if sym & 0x04: flags |= 0x01  # Scroll
    if sym & 0x08: flags |= 0x08  # Kana
    return flags


# ── Device discovery ─────────────────────────────────────────────────

def find_keyboards():
    """All keyboard endpoints (usage page 1, usage 6) of the K10 Pro."""
    import hid
    return [
        d for d in hid.enumerate()
        if d.get('vendor_id') == KEYCHRON_VID
        and d.get('usage_page') == 1
        and d.get('usage') == 6
    ]


def path_str(path):
    return path.decode() if isinstance(path, bytes) else path


# ── macOS: HID output report ─────────────────────────────────────────

class MacLink:
    def __init__(self, keyboards):
        import hid
        path = keyboards[0]['path']
        if hasattr(hid, 'Device'):
            self.dev = hid.Device(path=path)
        else:
            self.dev = hid.device()
            self.dev.open_path(path if isinstance(path, bytes) else path.encode())

    def send(self, sym):
        self.dev.write(bytes([REPORT_ID, symbol_to_hid(sym)]))

    def close(self):
        try:
            self.dev.close()
        except Exception:
            pass


# ── Windows: IOCTL_KEYBOARD_SET_INDICATORS on \KBD endpoints ────────

class WinLink:
    GENERIC_WRITE = 0x40000000
    FILE_SHARE_RW = 0x03
    OPEN_EXISTING = 3
    IOCTL_SET     = 0x000B0008
    VK_CAPITAL    = 0x14
    VK_NUMLOCK    = 0x90
    VK_SCROLL     = 0x91

    def __init__(self, keyboards):
        import ctypes
        self.ctypes = ctypes
        k32 = ctypes.WinDLL('kernel32', use_last_error=True)
        k32.CreateFileW.restype = ctypes.c_void_p
        k32.CreateFileW.argtypes = [
            ctypes.c_wchar_p, ctypes.c_ulong, ctypes.c_ulong,
            ctypes.c_void_p, ctypes.c_ulong, ctypes.c_ulong, ctypes.c_void_p,
        ]
        k32.DeviceIoControl.restype = ctypes.c_int
        k32.DeviceIoControl.argtypes = [
            ctypes.c_void_p, ctypes.c_ulong, ctypes.c_void_p, ctypes.c_ulong,
            ctypes.c_void_p, ctypes.c_ulong, ctypes.POINTER(ctypes.c_ulong), ctypes.c_void_p,
        ]
        k32.CloseHandle.restype = ctypes.c_int
        k32.CloseHandle.argtypes = [ctypes.c_void_p]
        self.k32 = k32
        self.user32 = ctypes.WinDLL('user32')

        invalid = ctypes.c_void_p(-1).value
        self.handles = []
        for kb in keyboards:
            path = path_str(kb['path'])
            if not path.upper().endswith('\\KBD'):
                path += '\\KBD'
            h = k32.CreateFileW(path, self.GENERIC_WRITE, self.FILE_SHARE_RW,
                                None, self.OPEN_EXISTING, 0, None)
            if h is not None and h != invalid:
                self.handles.append(h)
        if not self.handles:
            raise OSError(f"CreateFile failed on all keyboard endpoints "
                          f"(error {ctypes.get_last_error()})")

    def _set_flags(self, flags):
        import struct
        params = self.ctypes.create_string_buffer(struct.pack('<HH', 0, flags))
        returned = self.ctypes.c_ulong(0)
        ok_any = False
        for h in self.handles:
            if self.k32.DeviceIoControl(h, self.IOCTL_SET, params, 4, None, 0,
                                        self.ctypes.byref(returned), None):
                ok_any = True
        if not ok_any:
            raise OSError(f"DeviceIoControl failed (error {self.ctypes.get_last_error()})")

    def send(self, sym):
        self._set_flags(symbol_to_win_flags(sym))

    def restore(self):
        """Put the real Windows lock state back on the keyboard."""
        flags = 0
        if self.user32.GetKeyState(self.VK_SCROLL) & 1:  flags |= 0x01
        if self.user32.GetKeyState(self.VK_NUMLOCK) & 1: flags |= 0x02
        if self.user32.GetKeyState(self.VK_CAPITAL) & 1: flags |= 0x04
        self._set_flags(flags)

    def close(self):
        for h in self.handles:
            self.k32.CloseHandle(h)
        self.handles = []


# ── Command execution ────────────────────────────────────────────────

def run_frame(keyboards, frame):
    link = WinLink(keyboards) if IS_WINDOWS else MacLink(keyboards)
    try:
        last = len(frame) - 1
        for i, sym in enumerate(frame):
            try:
                link.send(sym)
            except Exception:
                # The trailing separator may fail if a BT switch already dropped the link
                if i != last:
                    raise
            time.sleep(STEP_DELAY)
        if IS_WINDOWS:
            try:
                link.restore()
            except Exception:
                pass
    finally:
        link.close()


def err(msg):
    print(msg, file=sys.stderr)


def print_status(keyboards):
    print(f"  Keyboard:  {keyboards[0].get('product_string') or '?'}")
    print(f"  Platform:  {platform.system()}")
    print(f"  Method:    {'IOCTL_KEYBOARD_SET_INDICATORS' if IS_WINDOWS else 'HID output report'}")
    for kb in keyboards:
        print(f"  Endpoint:  {path_str(kb['path'])}")


def main():
    args = [a for a in sys.argv[1:] if a not in ('-s', '--silent')]
    silent = len(args) != len(sys.argv) - 1

    if not args or args[0] in ('-h', '--help', 'help'):
        print(__doc__.strip())
        sys.exit(0)

    name = args[0].lower()

    try:
        keyboards = find_keyboards()
    except ImportError:
        err("Error: hid library missing "
            "(macOS: brew install hidapi && pip install hid; Windows: pip install hidapi)")
        sys.exit(EXIT_ERROR)

    if name == 'status':
        if not keyboards:
            err("K10 Pro not found. Is BT connected?")
            sys.exit(EXIT_ERROR)
        print_status(keyboards)
        sys.exit(0)

    if name not in COMMANDS:
        err(f"Unknown command: {name} (kc.py --help for the list)")
        sys.exit(EXIT_USAGE)

    code, label, takes_param = COMMANDS[name]

    value = None
    if takes_param:
        if len(args) < 2:
            err(f"Usage: kc.py {name} <1-31>")
            sys.exit(EXIT_USAGE)
        try:
            value = int(args[1])
        except ValueError:
            value = -1
        if not 1 <= value <= 31:
            err(f"Value must be 1-31, got: {args[1]}")
            sys.exit(EXIT_USAGE)

    if not keyboards:
        err("K10 Pro not found. Is BT connected?")
        sys.exit(EXIT_ERROR)

    try:
        run_frame(keyboards, build_frame(code, value))
    except Exception as e:
        msg = str(e)
        err(f"Error: {msg}")
        if not IS_WINDOWS and ('open' in msg.lower() or 'permission' in msg.lower()):
            err("  Run with sudo, or add the terminal app to")
            err("  System Settings → Privacy & Security → Input Monitoring")
        sys.exit(EXIT_ERROR)

    if not silent:
        print(f"✓ {label}" + (f" → {value}" if value is not None else ""))


if __name__ == '__main__':
    main()
