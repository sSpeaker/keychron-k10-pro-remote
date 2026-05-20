#!/usr/bin/env python3
"""
kc - Keychron K10 Pro remote control

Usage:
    sudo python3 kc.py bt1             Switch to BT device 1
    sudo python3 kc.py bt2             Switch to BT device 2
    sudo python3 kc.py bt3             Switch to BT device 3
    sudo python3 kc.py bat             Show battery level
    sudo python3 kc.py off             Disconnect BT
    sudo python3 kc.py rgb             Toggle RGB
    sudo python3 kc.py rgb_on          Turn on RGB
    sudo python3 kc.py rgb_off         Turn off RGB
    sudo python3 kc.py mac             Mac layout
    sudo python3 kc.py win             Windows layout
    sudo python3 kc.py sleep           Keyboard sleep
    sudo python3 kc.py mode <1-31>     Set RGB effect
    sudo python3 kc.py bright <1-31>   Set RGB brightness
    sudo python3 kc.py status          Connection info
"""

import sys
import time

MAGIC       = 0x18
REPORT_ID   = 0x01
STEP_DELAY  = 0.08
KEYCHRON_VID = 0x3434

# Commands (must match led_bt_control.h)
CMD = {
    'bt1':     (0x01, 'Switch to BT 1'),
    'bt2':     (0x02, 'Switch to BT 2'),
    'bt3':     (0x03, 'Switch to BT 3'),
    'bat':     (0x04, 'Battery level'),
    'off':     (0x05, 'Disconnect BT'),
    'rgb':     (0x06, 'RGB toggle'),
    'rgb_on':  (0x07, 'RGB on'),
    'rgb_off': (0x08, 'RGB off'),
    'mac':     (0x09, 'Mac layout'),
    'win':     (0x0A, 'Windows layout'),
    'sleep':   (0x0B, 'Keyboard sleep'),
    'mode':    (0x0C, 'RGB mode'),
    'bright':  (0x0D, 'RGB brightness'),
}

PARAM_CMDS = {'mode', 'bright'}

def find_keyboard():
    import hid
    for d in hid.enumerate():
        if d.get('vendor_id') == KEYCHRON_VID and d.get('usage_page') == 1 and d.get('usage') == 6:
            return d
    return None

def send(path, cmd_byte):
    import hid
    dev = hid.Device(path=path)
    try:
        dev.write(bytes([REPORT_ID, MAGIC]))
        time.sleep(STEP_DELAY)
        dev.write(bytes([REPORT_ID, cmd_byte]))
        time.sleep(STEP_DELAY)
        try:
            dev.write(bytes([REPORT_ID, 0x00]))
        except Exception:
            pass
    finally:
        try:
            dev.close()
        except Exception:
            pass

def send_with_value(path, cmd_byte, value):
    import hid
    dev = hid.Device(path=path)
    try:
        dev.write(bytes([REPORT_ID, MAGIC]))
        time.sleep(STEP_DELAY)
        dev.write(bytes([REPORT_ID, cmd_byte]))
        time.sleep(STEP_DELAY)
        dev.write(bytes([REPORT_ID, value]))
        time.sleep(STEP_DELAY)
        try:
            dev.write(bytes([REPORT_ID, 0x00]))
        except Exception:
            pass
    finally:
        try:
            dev.close()
        except Exception:
            pass

def main():
    if len(sys.argv) < 2 or sys.argv[1] in ('-h', '--help', 'help'):
        print(__doc__.strip())
        sys.exit(0)

    arg = sys.argv[1].lower()

    if arg == 'status':
        kb = find_keyboard()
        if kb:
            print(f"  Keyboard:  {kb.get('product_string', '?')}")
            print(f"  Path:      {kb['path']}")
            print(f"  Transport: BLE HID")
        else:
            print("  Keyboard not found. Is BT connected?")
        sys.exit(0)

    if arg not in CMD:
        print(f"Unknown command: {arg}")
        print()
        print(__doc__.strip())
        sys.exit(1)

    cmd_byte, label = CMD[arg]

    value = None
    if arg in PARAM_CMDS:
        if len(sys.argv) < 3:
            print(f"Usage: kc.py {arg} <1-31>")
            sys.exit(1)
        try:
            value = int(sys.argv[2])
            assert 1 <= value <= 31
        except (ValueError, AssertionError):
            print(f"Value must be 1-31, got: {sys.argv[2]}")
            sys.exit(1)

    kb = find_keyboard()
    if not kb:
        print("K10 Pro not found. Is BT connected?")
        sys.exit(1)

    try:
        if value is not None:
            send_with_value(kb['path'], cmd_byte, value)
            print(f"✓ {label} → {value}")
        else:
            send(kb['path'], cmd_byte)
            print(f"✓ {label}")
    except Exception as e:
        print(f"Error: {e}")
        sys.exit(1)

if __name__ == '__main__':
    main()
