// SPDX-License-Identifier: GPL-3.0-or-later

// Package output emits keyboard and mouse events through virtual uinput devices.
package output

import (
	"errors"
	"fmt"
	"slices"

	"github.com/holoplot/go-evdev"
)

var mouseButtons = []evdev.EvCode{
	evdev.BTN_LEFT, evdev.BTN_RIGHT, evdev.BTN_MIDDLE, evdev.BTN_SIDE, evdev.BTN_EXTRA,
}

var mouseAxes = []evdev.EvCode{
	evdev.REL_X, evdev.REL_Y,
	evdev.REL_WHEEL, evdev.REL_HWHEEL,
	evdev.REL_WHEEL_HI_RES, evdev.REL_HWHEEL_HI_RES,
}

// Output owns a virtual keyboard and a virtual mouse, and remembers every
// key currently held so they can all be released on shutdown or pause.
type Output struct {
	kbd   *evdev.InputDevice
	mouse *evdev.InputDevice
	held  map[evdev.EvCode]bool

	// Hi-res scroll not yet converted to legacy wheel notches.
	wheelRest, hwheelRest int32
}

// HiResPerNotch is the number of REL_*_HI_RES units in one legacy wheel notch.
const HiResPerNotch = 120

// New creates the virtual devices. It needs write access to /dev/uinput.
func New() (*Output, error) {
	// Keyboard: every key that Supports, mouse buttons excepted.
	var keys []evdev.EvCode
	for c := evdev.EvCode(1); c < evdev.BTN_DPAD_UP; c++ {
		if Supports(c) && !isMouseButton(c) {
			keys = append(keys, c)
		}
	}
	kbd, err := evdev.CreateDevice("gamepad-desktop keyboard",
		evdev.InputID{BusType: 0x06, Vendor: 0x1, Product: 0x1, Version: 1},
		map[evdev.EvType][]evdev.EvCode{evdev.EV_KEY: keys})
	if err != nil {
		return nil, fmt.Errorf("creating virtual keyboard: %w", err)
	}

	mouse, err := evdev.CreateDevice("gamepad-desktop mouse",
		evdev.InputID{BusType: 0x06, Vendor: 0x1, Product: 0x2, Version: 1},
		map[evdev.EvType][]evdev.EvCode{evdev.EV_KEY: mouseButtons, evdev.EV_REL: mouseAxes})
	if err != nil {
		kbd.Close()
		return nil, fmt.Errorf("creating virtual mouse: %w", err)
	}

	return &Output{kbd: kbd, mouse: mouse, held: map[evdev.EvCode]bool{}}, nil
}

func isMouseButton(code evdev.EvCode) bool {
	return code >= evdev.BTN_MOUSE && code < evdev.BTN_JOYSTICK
}

// Supports reports whether code can be emitted by the virtual devices:
// the KEY_* codes below BTN_MISC and above the buttons, and mouseButtons.
func Supports(code evdev.EvCode) bool {
	return (code >= 1 && code < evdev.BTN_MISC) ||
		(code >= evdev.KEY_OK && code < evdev.BTN_DPAD_UP) ||
		slices.Contains(mouseButtons, code)
}

func (o *Output) deviceFor(code evdev.EvCode) *evdev.InputDevice {
	if isMouseButton(code) {
		return o.mouse
	}
	return o.kbd
}

func emit(dev *evdev.InputDevice, typ evdev.EvType, code evdev.EvCode, value int32) error {
	return emitFrame(dev, evdev.InputEvent{Type: typ, Code: code, Value: value})
}

// emitFrame writes events followed by a single SYN_REPORT, so they are
// delivered as one atomic update.
func emitFrame(dev *evdev.InputDevice, events ...evdev.InputEvent) error {
	for i := range events {
		if err := dev.WriteOne(&events[i]); err != nil {
			return err
		}
	}
	return dev.WriteOne(&evdev.InputEvent{Type: evdev.EV_SYN, Code: evdev.SYN_REPORT})
}

func rel(code evdev.EvCode, v int32) evdev.InputEvent {
	return evdev.InputEvent{Type: evdev.EV_REL, Code: code, Value: v}
}

// Move moves the pointer by (dx, dy) pixels.
func (o *Output) Move(dx, dy int32) error {
	var evs []evdev.InputEvent
	if dx != 0 {
		evs = append(evs, rel(evdev.REL_X, dx))
	}
	if dy != 0 {
		evs = append(evs, rel(evdev.REL_Y, dy))
	}
	if len(evs) == 0 {
		return nil
	}
	return emitFrame(o.mouse, evs...)
}

// Scroll scrolls by v (positive = up) and h (positive = right), in hi-res
// units (HiResPerNotch per notch). Legacy REL_WHEEL/REL_HWHEEL notches are
// emitted alongside for applications that ignore hi-res events.
func (o *Output) Scroll(v, h int32) error {
	var evs []evdev.InputEvent
	if v != 0 {
		evs = append(evs, rel(evdev.REL_WHEEL_HI_RES, v))
		o.wheelRest += v
		if n := o.wheelRest / HiResPerNotch; n != 0 {
			evs = append(evs, rel(evdev.REL_WHEEL, n))
			o.wheelRest -= n * HiResPerNotch
		}
	}
	if h != 0 {
		evs = append(evs, rel(evdev.REL_HWHEEL_HI_RES, h))
		o.hwheelRest += h
		if n := o.hwheelRest / HiResPerNotch; n != 0 {
			evs = append(evs, rel(evdev.REL_HWHEEL, n))
			o.hwheelRest -= n * HiResPerNotch
		}
	}
	if len(evs) == 0 {
		return nil
	}
	return emitFrame(o.mouse, evs...)
}

// Key presses (down=true) or releases a key or mouse button.
func (o *Output) Key(code evdev.EvCode, down bool) error {
	var v int32
	if down {
		v = 1
		o.held[code] = true
	} else {
		delete(o.held, code)
	}
	return emit(o.deviceFor(code), evdev.EV_KEY, code, v)
}

// ReleaseAll releases every key and button still held.
func (o *Output) ReleaseAll() error {
	var errs []error
	for code := range o.held {
		errs = append(errs, o.Key(code, false))
	}
	return errors.Join(errs...)
}

// Close releases held keys and destroys the virtual devices.
func (o *Output) Close() error {
	return errors.Join(o.ReleaseAll(), o.kbd.Close(), o.mouse.Close())
}
