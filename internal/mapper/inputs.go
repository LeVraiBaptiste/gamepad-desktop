// SPDX-License-Identifier: GPL-3.0-or-later

package mapper

import (
	"github.com/holoplot/go-evdev"
)

// Button is a press or release of a gamepad button, including the virtual
// buttons derived from the d-pad hat and the analog triggers.
type Button struct {
	Code evdev.EvCode
	Down bool
}

// triggerHysteresis is how far below the threshold a trigger must fall to
// release, so a trigger held near the threshold does not chatter.
const triggerHysteresis = 0.1

var triggers = map[evdev.EvCode]evdev.EvCode{
	evdev.ABS_Z:  evdev.BTN_TL2,
	evdev.ABS_RZ: evdev.BTN_TR2,
}

// Inputs turns raw gamepad events into Button events.
type Inputs struct {
	abs       map[evdev.EvCode]evdev.AbsInfo
	threshold float64
	hatX      int32
	hatY      int32
	triggered map[evdev.EvCode]bool
}

// NewInputs creates a translator for a gamepad with the given axis ranges.
// threshold is the fraction of trigger travel at which a trigger presses.
func NewInputs(abs map[evdev.EvCode]evdev.AbsInfo, threshold float64) *Inputs {
	return &Inputs{abs: abs, threshold: threshold, triggered: map[evdev.EvCode]bool{}}
}

// Translate returns the button changes caused by ev. Events that are not
// buttons, d-pad or triggers (sticks, sync) yield nothing.
func (in *Inputs) Translate(ev *evdev.InputEvent) []Button {
	switch ev.Type {
	case evdev.EV_KEY:
		if ev.Value == 2 { // autorepeat
			return nil
		}
		return []Button{{ev.Code, ev.Value != 0}}
	case evdev.EV_ABS:
		switch ev.Code {
		case evdev.ABS_HAT0X:
			return hat(&in.hatX, ev.Value, evdev.BTN_DPAD_LEFT, evdev.BTN_DPAD_RIGHT)
		case evdev.ABS_HAT0Y:
			return hat(&in.hatY, ev.Value, evdev.BTN_DPAD_UP, evdev.BTN_DPAD_DOWN)
		}
		if btn, ok := triggers[ev.Code]; ok {
			return in.trigger(ev.Code, btn, ev.Value)
		}
	}
	return nil
}

// hat converts a hat axis (-1, 0, 1) into presses of neg and pos.
func hat(state *int32, v int32, neg, pos evdev.EvCode) []Button {
	v = max(-1, min(1, v))
	old := *state
	if v == old {
		return nil
	}
	*state = v
	var out []Button
	switch old {
	case -1:
		out = append(out, Button{neg, false})
	case 1:
		out = append(out, Button{pos, false})
	}
	switch v {
	case -1:
		out = append(out, Button{neg, true})
	case 1:
		out = append(out, Button{pos, true})
	}
	return out
}

func (in *Inputs) trigger(axis, btn evdev.EvCode, v int32) []Button {
	info, ok := in.abs[axis]
	if !ok || info.Maximum <= info.Minimum {
		return nil
	}
	pos := float64(v-info.Minimum) / float64(info.Maximum-info.Minimum)
	pressed := in.triggered[axis]
	switch {
	case !pressed && pos >= in.threshold:
		in.triggered[axis] = true
		return []Button{{btn, true}}
	case pressed && pos < in.threshold-triggerHysteresis:
		in.triggered[axis] = false
		return []Button{{btn, false}}
	}
	return nil
}
