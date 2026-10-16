/* LED-Based Remote Control - Implementation (protocol v2)
 *
 * Frame parser (runs in led_update_user):
 *
 *   IDLE --15--> CMD
 *   CMD  --cmd--> END              simple command, queued
 *   CMD  --cmd--> HI --hi--> LO --lo--> END   parameter command, queued
 *   END  --0----> IDLE             trailing separator swallowed
 *
 *   0 inside a frame is a separator and is ignored.
 *   15 inside a frame restarts the frame.
 *   A byte with the Compose bit set, or a gap longer than
 *   LED_CMD_WINDOW_MS, aborts the frame.
 *
 * Queued commands run DEFERRED_EXEC_MS later from led_bt_control_task()
 * (matrix_scan_user context). The delay lets the host finish the frame
 * and restore its lock state before a BT switch drops the connection,
 * and QMK RGB functions only behave correctly from the main loop.
 *
 * Outside a frame, every LED report is the host's real lock state. It is
 * remembered in real_led, and os_state_indicate() is overridden to draw
 * the Caps/Num Lock indicators from real_led, so frame bytes (magic 15
 * includes Caps and Num) never flash the indicators.
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
#    include "indicator.h"
#endif

#ifdef RGB_MATRIX_ENABLE
#    include "rgb_matrix.h"
#endif

#define MAC_BASE 0
#define WIN_BASE 2

#define DEFERRED_EXEC_MS  300
#define WAKE_TAP_DELAY_MS 500

#define HID_COMPOSE_BIT 0x08

typedef enum { ST_IDLE, ST_CMD, ST_HI, ST_LO, ST_END } frame_state_t;

static frame_state_t frame_state = ST_IDLE;
static uint32_t      frame_time  = 0; /* time of the last frame byte */
static uint8_t       frame_cmd   = 0;
static uint8_t       frame_hi    = 0;

static led_t    real_led      = {0};
static uint8_t  pending_cmd   = 0;
static uint8_t  pending_value = 0;
static uint32_t pending_time  = 0;
static uint32_t wake_tap_time = 0;

static uint8_t led_to_raw(led_t s) {
    uint8_t raw = 0;
    if (s.num_lock)    raw |= (1 << 0);
    if (s.caps_lock)   raw |= (1 << 1);
    if (s.scroll_lock) raw |= (1 << 2);
    if (s.compose)     raw |= (1 << 3);
    if (s.kana)        raw |= (1 << 4);
    return raw;
}

/* HID bits {0,1,2,4} -> 4-bit symbol */
static uint8_t raw_to_symbol(uint8_t raw) {
    return (raw & 0x07) | ((raw & 0x10) >> 1);
}

static bool is_param_command(uint8_t cmd) {
    return cmd == LED_CMD_SET_MODE || cmd == LED_CMD_SET_BRIGHT;
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
#    ifdef RGB_MATRIX_ENABLE
            /* Physically turn the LED driver off before the MCU stops;
             * rgb_matrix_task() exits shutdown on the next flush after wakeup */
            rgb_matrix_driver_shutdown();
#    endif
            enter_power_mode(LOW_POWER_MODE);
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
            /* rgb_matrix_mode() is ignored while RGB is disabled */
            if (!rgb_matrix_is_enabled()) rgb_matrix_toggle();
            rgb_matrix_mode(value);
            break;

        case LED_CMD_SET_BRIGHT: {
            uint8_t level = value > 31 ? 31 : value;
            if (!rgb_matrix_is_enabled()) rgb_matrix_toggle();
            rgb_matrix_sethsv(rgb_matrix_get_hue(), rgb_matrix_get_sat(),
                              (uint8_t)((uint16_t)level * 255 / 31));
            break;
        }
#endif

        /* Same as the Mac/Win dip switch: the default layer must change,
         * layer_move() alone loses to a higher default layer */
        case LED_CMD_MAC_LAYER:
            layer_clear();
            default_layer_set(1UL << MAC_BASE);
            break;

        case LED_CMD_WIN_LAYER:
            layer_clear();
            default_layer_set(1UL << WIN_BASE);
            break;

        default:
            break;
    }
}

bool led_bt_control_process(led_t led_state) {
    uint8_t raw = led_to_raw(led_state);
    uint8_t sym = raw_to_symbol(raw);

    if (frame_state != ST_IDLE) {
        bool timed_out = timer_elapsed32(frame_time) > LED_CMD_WINDOW_MS;
        if (timed_out || (raw & HID_COMPOSE_BIT)) {
            frame_state = ST_IDLE; /* abort, handle this byte as a normal report */
        }
    }

    if (frame_state == ST_IDLE) {
        if (!(raw & HID_COMPOSE_BIT) && sym == LED_SYM_MAGIC) {
            frame_state = ST_CMD;
            frame_time  = timer_read32();
            return false;
        }
        real_led = led_state;
        return true;
    }

    frame_time = timer_read32();

    if (sym == LED_SYM_MAGIC) {
        frame_state = ST_CMD; /* restart */
        return false;
    }

    if (sym == LED_SYM_SEPARATOR) {
        if (frame_state == ST_END) {
            frame_state = ST_IDLE; /* frame closed */
        }
        return false;
    }

    switch (frame_state) {
        case ST_CMD:
            if (is_param_command(sym)) {
                frame_cmd   = sym;
                frame_state = ST_HI;
            } else {
                queue_command(sym, 0);
                frame_state = ST_END;
            }
            break;

        case ST_HI:
            frame_hi    = sym;
            frame_state = ST_LO;
            break;

        case ST_LO:
            queue_command(frame_cmd, (uint8_t)((frame_hi - 1) * LED_SYM_DIGITS + (sym - 1)));
            frame_state = ST_END;
            break;

        default: /* data byte where the trailing separator was expected */
            frame_state = ST_IDLE;
            real_led    = led_state;
            return true;
    }

    return false;
}

void led_bt_control_task(void) {
    if (frame_state != ST_IDLE && timer_elapsed32(frame_time) > LED_CMD_WINDOW_MS) {
        frame_state = ST_IDLE;
    }

    if (pending_time && timer_elapsed32(pending_time) > DEFERRED_EXEC_MS) {
        run_command();
    }

    /* After a BT switch, a phantom F24 tap activates the new connection */
    if (wake_tap_time && timer_elapsed32(wake_tap_time) > WAKE_TAP_DELAY_MS) {
        wake_tap_time = 0;
        tap_code(KC_F24);
    }
}

#if defined(KC_BLUETOOTH_ENABLE) && defined(RGB_MATRIX_ENABLE)
/* Overrides the weak os_state_indicate() in keychron/bluetooth/indicator.c:
 * same logic, but driven by the real host lock state instead of the last
 * raw LED report, which may be a frame byte. */
void os_state_indicate(void) {
#    if defined(NUM_LOCK_INDEX)
    if (real_led.num_lock) {
        rgb_matrix_set_color(NUM_LOCK_INDEX, 255, 255, 255);
    }
#    endif
#    if defined(CAPS_LOCK_INDEX)
    if (real_led.caps_lock) {
#        if defined(DIM_CAPS_LOCK)
        rgb_matrix_set_color(CAPS_LOCK_INDEX, 0, 0, 0);
#        else
        rgb_matrix_set_color(CAPS_LOCK_INDEX, 255, 255, 255);
#        endif
    }
#    endif
#    if defined(SCROLL_LOCK_INDEX)
    if (real_led.scroll_lock) {
        rgb_matrix_set_color(SCROLL_LOCK_INDEX, 255, 255, 255);
    }
#    endif
#    if defined(COMPOSE_LOCK_INDEX)
    if (real_led.compose) {
        rgb_matrix_set_color(COMPOSE_LOCK_INDEX, 255, 255, 255);
    }
#    endif
#    if defined(KANA_LOCK_INDEX)
    if (real_led.kana) {
        rgb_matrix_set_color(KANA_LOCK_INDEX, 255, 255, 255);
    }
#    endif
}
#endif
