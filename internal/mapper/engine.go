// SPDX-License-Identifier: GPL-3.0-or-later

// Package mapper turns gamepad buttons into keyboard and mouse output
// according to the active profile.
package mapper

import (
	"errors"
	"slices"
	"time"

	"github.com/holoplot/go-evdev"

	"github.com/LeVraiBaptiste/gamepad-desktop/internal/config"
)

// Sink receives key and button output.
type Sink interface {
	Key(code evdev.EvCode, down bool) error
}

// Engine applies profile actions to button events.
type Engine struct {
	out           Sink
	sequenceDelay time.Duration
	// held maps each pressed gamepad button to the action it started, so
	// the release undoes that action even if the profile changed meanwhile.
	held map[evdev.EvCode]config.Action
	// altTab is the gamepad button holding Alt for the window switcher,
	// or 0 when the switcher is not open.
	altTab evdev.EvCode
	// pressed is the set of gamepad buttons physically down, for combos.
	pressed map[evdev.EvCode]bool
	// keys counts, for each output key, how many actions hold it down, so
	// a key shared by two actions (say Ctrl, or the Alt of alt-tab) stays
	// down until the last of them releases it.
	keys map[evdev.EvCode]int
	// paused is set by a pause action: nothing is emitted until the same
	// action of pausedBy is triggered again.
	paused   bool
	pausedBy *config.Profile

	// OnPause, when set, is called after pausing or resuming.
	OnPause func(paused bool)

	// Logf, when set, receives a line per action.
	Logf func(format string, args ...any)
	// Sleep waits between sequence steps; replaced in tests.
	Sleep func(time.Duration)
}

// NewEngine creates an engine writing to out.
func NewEngine(out Sink, sequenceDelay time.Duration) *Engine {
	return &Engine{
		out:           out,
		sequenceDelay: sequenceDelay,
		held:          map[evdev.EvCode]config.Action{},
		pressed:       map[evdev.EvCode]bool{},
		keys:          map[evdev.EvCode]int{},
		Sleep:         time.Sleep,
	}
}

func (e *Engine) logf(format string, args ...any) {
	if e.Logf != nil {
		e.Logf(format, args...)
	}
}

// Paused reports whether a pause action is in effect.
func (e *Engine) Paused() bool { return e.paused }

// Handle applies a button event. p is the profile active at press time; it
// is ignored on release and while paused (pass nil then).
func (e *Engine) Handle(b Button, p *config.Profile) error {
	if !b.Down {
		delete(e.pressed, b.Code)
		return e.release(b.Code)
	}
	if e.pressed[b.Code] {
		return nil // already down
	}
	e.pressed[b.Code] = true

	if e.paused {
		// Only the pause action can do something: resume.
		if a, label, ok := e.lookup(b.Code, e.pausedBy); ok && a.Kind == config.Pause {
			e.logf("  → [%s] %s: resume", e.pausedBy.Name, label)
			return e.setPaused(false, nil)
		}
		return nil
	}
	if e.altTab != 0 {
		if chord, ok := switcherKeys[b.Code]; ok {
			e.logf("  → alt-tab: %s", evdev.CodeName(evdev.EV_KEY, chord[len(chord)-1]))
			return e.tap(chord)
		}
	}
	a, label, ok := e.lookup(b.Code, p)
	if !ok {
		e.logf("  → unmapped in [%s]", p.Name)
		return nil
	}
	e.logf("  → [%s] %s: %s", p.Name, label, a.Source)

	switch a.Kind {
	case config.Keys:
		if len(a.Steps) == 1 {
			// A single chord is held for as long as the button is.
			e.held[b.Code] = a
			return e.press(a.Steps[0])
		}
		for i, chord := range a.Steps {
			if i > 0 {
				e.Sleep(e.sequenceDelay)
			}
			if err := e.tap(chord); err != nil {
				return err
			}
		}
	case config.AltTab:
		if e.altTab != 0 {
			return nil // already switching with another button
		}
		// Alt stays down while the button is held, like a held Alt key:
		// the d-pad then moves in the switcher and the release picks.
		e.altTab = b.Code
		e.held[b.Code] = altHold
		if err := e.press(altHold.Steps[0]); err != nil {
			return err
		}
		return e.tap(switcherKeys[evdev.BTN_DPAD_RIGHT])
	case config.Pause:
		return e.setPaused(true, p)
	}
	return nil
}

// lookup returns the action for pressing code: a combo completed by code
// (the one with the most buttons wins, ties going to the lowest buttons so
// the choice does not depend on map order), else the button's own action.
func (e *Engine) lookup(code evdev.EvCode, p *config.Profile) (config.Action, string, bool) {
	var best *config.Combo
	for _, c := range p.Combos {
		if len(c.Buttons) <= 1 || !slices.Contains(c.Buttons, code) || (best != nil && !beats(c.Buttons, best.Buttons)) {
			continue
		}
		complete := true
		for _, other := range c.Buttons {
			if !e.pressed[other] {
				complete = false
				break
			}
		}
		if complete {
			best = &c
		}
	}
	if best != nil {
		return best.Action, best.Input, true
	}
	a, ok := p.Buttons[code]
	return a, evdev.CodeName(evdev.EV_KEY, code), ok
}

// beats reports whether a combo of buttons a takes precedence over one of b:
// more buttons first, then the lowest codes (both are sorted).
func beats(a, b []evdev.EvCode) bool {
	if len(a) != len(b) {
		return len(a) > len(b)
	}
	return slices.Compare(a, b) < 0
}

func (e *Engine) setPaused(paused bool, by *config.Profile) error {
	var err error
	if paused {
		err = e.ReleaseAll()
	}
	e.paused, e.pausedBy = paused, by
	if e.OnPause != nil {
		e.OnPause(paused)
	}
	return err
}

var altHold = config.Action{Kind: config.Keys, Steps: [][]evdev.EvCode{{evdev.KEY_LEFTALT}}, Source: "alt-tab"}

// switcherKeys are the keys sent by the d-pad while the switcher is open.
var switcherKeys = map[evdev.EvCode][]evdev.EvCode{
	evdev.BTN_DPAD_RIGHT: {evdev.KEY_TAB},
	evdev.BTN_DPAD_DOWN:  {evdev.KEY_TAB},
	evdev.BTN_DPAD_LEFT:  {evdev.KEY_LEFTSHIFT, evdev.KEY_TAB},
	evdev.BTN_DPAD_UP:    {evdev.KEY_LEFTSHIFT, evdev.KEY_TAB},
}

// tap presses then releases a chord.
func (e *Engine) tap(chord []evdev.EvCode) error {
	if err := e.press(chord); err != nil {
		return err
	}
	return e.releaseChord(chord)
}

func (e *Engine) press(chord []evdev.EvCode) error {
	for _, k := range chord {
		if err := e.key(k, true); err != nil {
			return err
		}
	}
	return nil
}

func (e *Engine) releaseChord(chord []evdev.EvCode) error {
	for _, k := range slices.Backward(chord) {
		if err := e.key(k, false); err != nil {
			return err
		}
	}
	return nil
}

// key presses or releases an output key, only emitting the first press and
// the last release when several actions hold it. A key tapped while another
// action holds it is therefore not repeated.
func (e *Engine) key(k evdev.EvCode, down bool) error {
	n := e.keys[k]
	switch {
	case down:
		e.keys[k] = n + 1
		if n > 0 {
			return nil
		}
	case n == 0:
		return nil
	case n > 1:
		e.keys[k] = n - 1
		return nil
	default:
		delete(e.keys, k)
	}
	return e.out.Key(k, down)
}

func (e *Engine) release(code evdev.EvCode) error {
	a, ok := e.held[code]
	if !ok {
		return nil
	}
	delete(e.held, code)
	if code == e.altTab {
		e.altTab = 0
	}
	return e.releaseChord(a.Steps[0])
}

// Disconnect forgets the buttons of a gamepad that went away and releases
// everything they held. A pause stays in effect.
func (e *Engine) Disconnect() error {
	clear(e.pressed)
	return e.ReleaseAll()
}

// ReleaseAll releases every action still held.
func (e *Engine) ReleaseAll() error {
	var errs []error
	for code := range e.held {
		errs = append(errs, e.release(code))
	}
	return errors.Join(errs...)
}
