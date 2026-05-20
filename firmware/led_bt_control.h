/* LED-Based Remote Control for Keychron K10 Pro
 *
 * Protocol:
 *   magic (0x18) → command (5-bit) → clear (0x00)
 *   magic (0x18) → param cmd (5-bit) → value (5-bit) → clear (0x00)
 *
 * Copyright 2024 - GPL-2.0-or-later
 */

#pragma once

#include <stdint.h>
#include <stdbool.h>
#include "led.h"

#define LED_CMD_WINDOW_MS        300
#define LED_CMD_MAGIC            0x18
#define LED_CMD_MAGIC_MASK       0x18
#define LED_CMD_5BIT_MASK        0x1F

/* Commands (5-bit, 0x01-0x1F) */
#define LED_CMD_BT_DEV1          0x01
#define LED_CMD_BT_DEV2          0x02
#define LED_CMD_BT_DEV3          0x03
#define LED_CMD_BAT_LEVEL        0x04
#define LED_CMD_BT_DISCONNECT    0x05
#define LED_CMD_RGB_TOGGLE       0x06
#define LED_CMD_RGB_ON           0x07
#define LED_CMD_RGB_OFF          0x08
#define LED_CMD_MAC_LAYER        0x09
#define LED_CMD_WIN_LAYER        0x0A
#define LED_CMD_SLEEP            0x0B
#define LED_CMD_SET_MODE         0x0C  /* +value byte */
#define LED_CMD_SET_BRIGHT       0x0D  /* +value byte */

bool led_bt_control_process(led_t led_state);
void led_bt_control_task(void);
