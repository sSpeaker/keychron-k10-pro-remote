/* LED-Based Remote Control for Keychron K10 Pro — protocol v2
 *
 * The host sends commands through HID LED output reports. Only four LED
 * bits are used, because Windows can drive Num/Caps/Scroll/Kana but has no
 * way to set Compose:
 *
 *   symbol bit 0 -> HID bit 0 (Num Lock)
 *   symbol bit 1 -> HID bit 1 (Caps Lock)
 *   symbol bit 2 -> HID bit 2 (Scroll Lock)
 *   symbol bit 3 -> HID bit 4 (Kana)
 *   HID bit 3 (Compose) must be 0 inside a frame
 *
 * QMK only reports LED changes, so two equal bytes in a row are lost.
 * Every frame therefore separates symbols with 0:
 *
 *   simple command: 15, cmd, 0
 *   with parameter: 15, cmd, 0, hi, 0, lo, 0
 *
 *   15     magic (HID 0x17), starts a frame
 *   cmd    command code 1..14
 *   hi/lo  parameter digits 1..14, value = (hi - 1) * 14 + (lo - 1), 0..195
 *   0      separator; the trailing 0 closes the frame
 *
 * Copyright 2024 - GPL-2.0-or-later
 */

#pragma once

#include <stdint.h>
#include <stdbool.h>
#include "led.h"

/* Max gap between two bytes of one frame */
#define LED_CMD_WINDOW_MS 300

#define LED_SYM_SEPARATOR 0
#define LED_SYM_MAGIC     15
#define LED_SYM_DIGITS    14 /* parameter digits are 1..14 */

/* Command codes (symbol values) */
#define LED_CMD_BT_DEV1       1
#define LED_CMD_BT_DEV2       2
#define LED_CMD_BT_DEV3       3
#define LED_CMD_BAT_LEVEL     4
#define LED_CMD_BT_DISCONNECT 5
#define LED_CMD_RGB_TOGGLE    6
#define LED_CMD_RGB_ON        7
#define LED_CMD_RGB_OFF       8
#define LED_CMD_MAC_LAYER     9
#define LED_CMD_WIN_LAYER     10
#define LED_CMD_SLEEP         11
#define LED_CMD_SET_MODE      12 /* + parameter */
#define LED_CMD_SET_BRIGHT    13 /* + parameter */

/* Call from led_update_user() */
bool led_bt_control_process(led_t led_state);

/* Call from matrix_scan_user() */
void led_bt_control_task(void);
