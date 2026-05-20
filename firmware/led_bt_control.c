/* LED-Based Remote Control - Implementation
 *
 * All commands are deferred to led_bt_control_task() which runs
 * in matrix_scan_user() — same context as keypress handling.
 *
 * Copyright 2024 - GPL-2.0-or-later
 */

#include "led_bt_control.h"
#include "k10_pro.h"
#include "timer.h"
#include "action_layer.h"

#ifdef KC_BLUETOOTH_ENABLE
#    include "bluetooth.h"
#    include "transport.h"
#    include "battery.h"
#    include "bat_level_animation.h"
#    include "lpm.h"
#endif

#ifdef RGB_MATRIX_ENABLE
#    include "rgb_matrix.h"
#endif

#define MAC_BASE 0
#define WIN_BASE 2

#define DEFERRED_EXEC_MS   200
#define WAKE_TAP_DELAY_MS  500

typedef enum { CMD_IDLE, CMD_ARMED, CMD_PARAM } cmd_state_t;

static cmd_state_t cmd_state      = CMD_IDLE;
static uint32_t    cmd_armed_time = 0;
static uint8_t     param_cmd      = 0;

static uint8_t     pending_cmd    = 0;
static uint8_t     pending_value  = 0;
static uint32_t    pending_time   = 0;
static uint32_t    wake_tap_time  = 0;

static uint8_t led_to_raw(led_t s) {
    return (s.num_lock)    << 0
         | (s.caps_lock)   << 1
         | (s.scroll_lock) << 2
         | (s.compose)     << 3
         | (s.kana)        << 4;
}

static void queue_command(uint8_t cmd, uint8_t value) {
    pending_cmd   = cmd;
    pending_value = value;
    pending_time  = timer_read32() | 1;
}

static void run_command(void) {
    uint8_t cmd   = pending_cmd;
    uint8_t value = pending_value;
    pending_cmd   = 0;
    pending_value = 0;
    pending_time  = 0;

    switch (cmd) {
#ifdef KC_BLUETOOTH_ENABLE
        case LED_CMD_BT_DEV1:
            bluetooth_connect_ex(1, 0);
            wake_tap_time = timer_read32() | 1;
            break;

        case LED_CMD_BT_DEV2:
            bluetooth_connect_ex(2, 0);
            wake_tap_time = timer_read32() | 1;
            break;

        case LED_CMD_BT_DEV3:
            bluetooth_connect_ex(3, 0);
            wake_tap_time = timer_read32() | 1;
            break;

        case LED_CMD_BT_DISCONNECT:
            bluetooth_disconnect();
            break;

        case LED_CMD_BAT_LEVEL:
            if (!usb_power_connected()) {
                bat_level_animiation_start(battery_get_percentage());
            }
            break;

        case LED_CMD_SLEEP:
#ifdef RGB_MATRIX_ENABLE
            rgb_matrix_driver_shutdown();
#endif
            enter_power_mode(LOW_POWER_MODE);
            /* After wakeup, rgb_matrix_task() will call exit_shutdown()
             * automatically on the next tick */
            break;
#endif

#ifdef RGB_MATRIX_ENABLE
        case LED_CMD_RGB_TOGGLE:
            rgb_matrix_toggle();
            break;

        case LED_CMD_RGB_ON:
            if (!rgb_matrix_is_enabled()) rgb_matrix_toggle();
            break;

        case LED_CMD_RGB_OFF:
            if (rgb_matrix_is_enabled()) rgb_matrix_toggle();
            break;

        case LED_CMD_SET_MODE:
            if (!rgb_matrix_is_enabled()) rgb_matrix_toggle();
            rgb_matrix_mode(value);
            break;

        case LED_CMD_SET_BRIGHT:
            if (!rgb_matrix_is_enabled()) rgb_matrix_toggle();
            rgb_matrix_sethsv(
                rgb_matrix_get_hue(),
                rgb_matrix_get_sat(),
                (uint8_t)((uint16_t)value * 255 / 31));
            break;
#endif

        case LED_CMD_MAC_LAYER:
            layer_move(MAC_BASE);
            break;

        case LED_CMD_WIN_LAYER:
            layer_move(WIN_BASE);
            break;

        default:
            break;
    }
}

bool led_bt_control_process(led_t led_state) {
    uint8_t raw = led_to_raw(led_state);
    uint8_t cmd = raw & LED_CMD_5BIT_MASK;

    switch (cmd_state) {
        case CMD_IDLE:
            if ((raw & LED_CMD_MAGIC_MASK) == LED_CMD_MAGIC) {
                cmd_state      = CMD_ARMED;
                cmd_armed_time = timer_read32();
                return false;
            }
            return true;

        case CMD_ARMED:
            if (timer_elapsed32(cmd_armed_time) > LED_CMD_WINDOW_MS) {
                cmd_state = CMD_IDLE;
                return true;
            }
            if (cmd == 0x00) {
                cmd_state = CMD_IDLE;
                return true;
            }
            if (cmd == LED_CMD_SET_MODE || cmd == LED_CMD_SET_BRIGHT) {
                param_cmd = cmd;
                cmd_state = CMD_PARAM;
            } else {
                queue_command(cmd, 0);
                cmd_state = CMD_IDLE;
            }
            return false;

        case CMD_PARAM:
            if (timer_elapsed32(cmd_armed_time) > LED_CMD_WINDOW_MS) {
                cmd_state = CMD_IDLE;
                param_cmd = 0;
                return true;
            }
            if (cmd == 0x00) {
                cmd_state = CMD_IDLE;
                param_cmd = 0;
                return true;
            }
            queue_command(param_cmd, cmd);
            param_cmd = 0;
            cmd_state = CMD_IDLE;
            return false;

        default:
            cmd_state = CMD_IDLE;
            return true;
    }
}

void led_bt_control_task(void) {
    if (cmd_state != CMD_IDLE && timer_elapsed32(cmd_armed_time) > LED_CMD_WINDOW_MS * 2) {
        cmd_state = CMD_IDLE;
        param_cmd = 0;
    }

    if (pending_time && timer_elapsed32(pending_time) > DEFERRED_EXEC_MS) {
        run_command();
    }

    if (wake_tap_time && timer_elapsed32(wake_tap_time) > WAKE_TAP_DELAY_MS) {
        wake_tap_time = 0;
        tap_code(KC_F24);
    }
}
