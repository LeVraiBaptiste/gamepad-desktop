// SPDX-License-Identifier: GPL-3.0-or-later

package mapper

import (
	"fmt"
	"slices"
	"testing"
	"time"

	"github.com/holoplot/go-evdev"

	"github.com/LeVraiBaptiste/gamepad-desktop/internal/config"
)

type recorder struct{ log []string }

func (r *recorder) Key(code evdev.EvCode, down bool) error {
	d := "up"
	if down {
		d = "down"
	}
	r.log = append(r.log, fmt.Sprintf("%s %s", evdev.KEYNames[code], d))
	return nil
}

func mustParse(t *testing.T, s string) *config.Config {
	t.Helper()
	cfg, err := config.Parse(s, "")
	if err != nil {
		t.Fatal(err)
	}
	return cfg
}

func newEngine() (*Engine, *recorder, *[]time.Duration) {
	r := &recorder{}
	e := NewEngine(r, 15*time.Millisecond)
	var sleeps []time.Duration
	e.Sleep = func(d time.Duration) { sleeps = append(sleeps, d) }
	return e, r, &sleeps
}

const testConfig = `
[default]
BTN_SOUTH = "BTN_LEFT"
BTN_NORTH = "ctrl+t"
DPAD_UP   = "ctrl+b, k"
[app.zen]
BTN_SOUTH = "KEY_ENTER"
`

func TestChordHeldUntilRelease(t *testing.T) {
	cfg := mustParse(t, testConfig)
	e, r, _ := newEngine()
	e.Handle(Button{evdev.BTN_NORTH, true}, cfg.Default)
	if want := []string{"KEY_LEFTCTRL down", "KEY_T down"}; !slices.Equal(r.log, want) {
		t.Fatalf("press: %v, want %v", r.log, want)
	}
	e.Handle(Button{evdev.BTN_NORTH, false}, cfg.Default)
	if want := []string{"KEY_LEFTCTRL down", "KEY_T down", "KEY_T up", "KEY_LEFTCTRL up"}; !slices.Equal(r.log, want) {
		t.Fatalf("release: %v, want %v", r.log, want)
	}
}

func TestSequence(t *testing.T) {
	cfg := mustParse(t, testConfig)
	e, r, sleeps := newEngine()
	e.Handle(Button{evdev.BTN_DPAD_UP, true}, cfg.Default)
	want := []string{"KEY_LEFTCTRL down", "KEY_B down", "KEY_B up", "KEY_LEFTCTRL up", "KEY_K down", "KEY_K up"}
	if !slices.Equal(r.log, want) {
		t.Fatalf("got %v, want %v", r.log, want)
	}
	if !slices.Equal(*sleeps, []time.Duration{15 * time.Millisecond}) {
		t.Errorf("sleeps = %v", *sleeps)
	}
	e.Handle(Button{evdev.BTN_DPAD_UP, false}, cfg.Default)
	if len(r.log) != len(want) {
		t.Errorf("release of a sequence emitted %v", r.log[len(want):])
	}
}

func TestReleaseUsesPressProfile(t *testing.T) {
	cfg := mustParse(t, testConfig)
	e, r, _ := newEngine()
	// Pressed in zen, focus moves to another app before release.
	e.Handle(Button{evdev.BTN_SOUTH, true}, cfg.Profile("zen"))
	e.Handle(Button{evdev.BTN_SOUTH, false}, cfg.Default)
	if want := []string{"KEY_ENTER down", "KEY_ENTER up"}; !slices.Equal(r.log, want) {
		t.Fatalf("got %v, want %v", r.log, want)
	}
}

func TestReleaseAll(t *testing.T) {
	cfg := mustParse(t, testConfig)
	e, r, _ := newEngine()
	e.Handle(Button{evdev.BTN_SOUTH, true}, cfg.Default)
	e.ReleaseAll()
	e.Handle(Button{evdev.BTN_SOUTH, false}, cfg.Default) // nothing left to release
	if want := []string{"BTN_MOUSE/BTN_LEFT down", "BTN_MOUSE/BTN_LEFT up"}; !slices.Equal(r.log, want) {
		t.Fatalf("got %v, want %v", r.log, want)
	}
}

func TestUnmappedIgnored(t *testing.T) {
	cfg := mustParse(t, testConfig)
	e, r, _ := newEngine()
	e.Handle(Button{evdev.BTN_WEST, true}, cfg.Default)
	e.Handle(Button{evdev.BTN_WEST, false}, cfg.Default)
	if len(r.log) != 0 {
		t.Fatalf("got %v", r.log)
	}
}

func abs(t evdev.EvType, c evdev.EvCode, v int32) *evdev.InputEvent {
	return &evdev.InputEvent{Type: t, Code: c, Value: v}
}

func TestHat(t *testing.T) {
	in := NewInputs(nil, 0.5)
	steps := []struct {
		v    int32
		want []Button
	}{
		{-1, []Button{{evdev.BTN_DPAD_LEFT, true}}},
		{-1, nil},
		{1, []Button{{evdev.BTN_DPAD_LEFT, false}, {evdev.BTN_DPAD_RIGHT, true}}},
		{0, []Button{{evdev.BTN_DPAD_RIGHT, false}}},
	}
	for i, s := range steps {
		if got := in.Translate(abs(evdev.EV_ABS, evdev.ABS_HAT0X, s.v)); !slices.Equal(got, s.want) {
			t.Errorf("step %d (%d): got %v, want %v", i, s.v, got, s.want)
		}
	}
	if got := in.Translate(abs(evdev.EV_ABS, evdev.ABS_HAT0Y, -1)); !slices.Equal(got, []Button{{evdev.BTN_DPAD_UP, true}}) {
		t.Errorf("hat up: %v", got)
	}
}

func TestTriggerHysteresis(t *testing.T) {
	in := NewInputs(map[evdev.EvCode]evdev.AbsInfo{evdev.ABS_Z: {Minimum: 0, Maximum: 255}}, 0.5)
	steps := []struct {
		v    int32
		want []Button
	}{
		{100, nil},                             // 0.39
		{130, []Button{{evdev.BTN_TL2, true}}}, // 0.51
		{255, nil},
		{110, nil},                              // 0.43: inside hysteresis band
		{100, []Button{{evdev.BTN_TL2, false}}}, // 0.39
		{0, nil},
	}
	for i, s := range steps {
		if got := in.Translate(abs(evdev.EV_ABS, evdev.ABS_Z, s.v)); !slices.Equal(got, s.want) {
			t.Errorf("step %d (%d): got %v, want %v", i, s.v, got, s.want)
		}
	}
	// No range info: ignored rather than misread.
	if got := in.Translate(abs(evdev.EV_ABS, evdev.ABS_RZ, 255)); got != nil {
		t.Errorf("unknown trigger range: %v", got)
	}
}

func TestKeyRepeatIgnored(t *testing.T) {
	in := NewInputs(nil, 0.5)
	if got := in.Translate(abs(evdev.EV_KEY, evdev.BTN_SOUTH, 2)); got != nil {
		t.Errorf("autorepeat: %v", got)
	}
}

const altTabConfig = `
[default]
BTN_TR     = "alt-tab"
BTN_TL     = "alt-tab"
DPAD_RIGHT = "KEY_RIGHT"
DPAD_LEFT  = "KEY_LEFT"
`

func TestAltTab(t *testing.T) {
	cfg := mustParse(t, altTabConfig)
	e, r, _ := newEngine()
	p := cfg.Default
	press := func(c evdev.EvCode) { e.Handle(Button{c, true}, p) }
	release := func(c evdev.EvCode) { e.Handle(Button{c, false}, p) }

	press(evdev.BTN_TR)
	press(evdev.BTN_DPAD_RIGHT)
	release(evdev.BTN_DPAD_RIGHT)
	press(evdev.BTN_DPAD_LEFT)
	release(evdev.BTN_DPAD_LEFT)
	press(evdev.BTN_TL) // a second alt-tab button does nothing while open
	release(evdev.BTN_TL)
	release(evdev.BTN_TR)

	want := []string{
		"KEY_LEFTALT down", "KEY_TAB down", "KEY_TAB up", // open on the next window
		"KEY_TAB down", "KEY_TAB up", // d-pad right: next
		"KEY_LEFTSHIFT down", "KEY_TAB down", "KEY_TAB up", "KEY_LEFTSHIFT up", // d-pad left: previous
		"KEY_LEFTALT up", // release picks the window
	}
	if !slices.Equal(r.log, want) {
		t.Fatalf("got  %v\nwant %v", r.log, want)
	}

	// Once closed, the d-pad is back to its profile mapping.
	r.log = nil
	press(evdev.BTN_DPAD_RIGHT)
	release(evdev.BTN_DPAD_RIGHT)
	if want := []string{"KEY_RIGHT down", "KEY_RIGHT up"}; !slices.Equal(r.log, want) {
		t.Fatalf("after alt-tab: got %v, want %v", r.log, want)
	}
}

func TestAltTabHeldDpadKeepsItsAction(t *testing.T) {
	cfg := mustParse(t, altTabConfig)
	e, r, _ := newEngine()
	p := cfg.Default
	// D-pad held before the switcher opens keeps its own action until released.
	e.Handle(Button{evdev.BTN_DPAD_RIGHT, true}, p)
	e.Handle(Button{evdev.BTN_TR, true}, p)
	e.Handle(Button{evdev.BTN_DPAD_RIGHT, false}, p)
	e.Handle(Button{evdev.BTN_TR, false}, p)
	want := []string{"KEY_RIGHT down", "KEY_LEFTALT down", "KEY_TAB down", "KEY_TAB up", "KEY_RIGHT up", "KEY_LEFTALT up"}
	if !slices.Equal(r.log, want) {
		t.Fatalf("got  %v\nwant %v", r.log, want)
	}
}

func TestAltTabReleaseAll(t *testing.T) {
	cfg := mustParse(t, altTabConfig)
	e, r, _ := newEngine()
	e.Handle(Button{evdev.BTN_TR, true}, cfg.Default)
	e.ReleaseAll() // e.g. gamepad unplugged mid-switch
	if last := r.log[len(r.log)-1]; last != "KEY_LEFTALT up" {
		t.Fatalf("Alt left down: %v", r.log)
	}
	r.log = nil
	e.Handle(Button{evdev.BTN_DPAD_RIGHT, true}, cfg.Default)
	if want := []string{"KEY_RIGHT down"}; !slices.Equal(r.log, want) {
		t.Fatalf("switcher still considered open: %v", r.log)
	}
}

const comboConfig = `
[default]
BTN_SOUTH = "BTN_LEFT"
BTN_START = "KEY_ENTER"
"BTN_SELECT+BTN_START" = "pause"
"BTN_TL+BTN_TR" = "ctrl+w"
"BTN_TL+BTN_TR+BTN_SOUTH" = "ctrl+shift+w"
`

func TestPause(t *testing.T) {
	cfg := mustParse(t, comboConfig)
	e, r, _ := newEngine()
	var events []bool
	e.OnPause = func(p bool) { events = append(events, p) }
	p := cfg.Default
	press := func(c evdev.EvCode) {
		if e.Paused() {
			e.Handle(Button{c, true}, nil)
		} else {
			e.Handle(Button{c, true}, p)
		}
	}
	release := func(c evdev.EvCode) { e.Handle(Button{c, false}, nil) }

	press(evdev.BTN_SOUTH) // held click is released by the pause
	press(evdev.BTN_SELECT)
	press(evdev.BTN_START) // completes the combo: no Enter
	release(evdev.BTN_START)
	release(evdev.BTN_SELECT)
	release(evdev.BTN_SOUTH)
	if want := []string{"BTN_MOUSE/BTN_LEFT down", "BTN_MOUSE/BTN_LEFT up"}; !slices.Equal(r.log, want) {
		t.Fatalf("pausing: got %v, want %v", r.log, want)
	}
	if !e.Paused() || !slices.Equal(events, []bool{true}) {
		t.Fatalf("not paused: %v", events)
	}

	// Paused: nothing is emitted, even the start of the combo.
	press(evdev.BTN_SOUTH)
	release(evdev.BTN_SOUTH)
	press(evdev.BTN_START)
	release(evdev.BTN_START)
	if len(r.log) != 2 {
		t.Fatalf("emitted while paused: %v", r.log[2:])
	}

	// The same combo resumes.
	press(evdev.BTN_SELECT)
	press(evdev.BTN_START)
	release(evdev.BTN_START)
	release(evdev.BTN_SELECT)
	if e.Paused() || !slices.Equal(events, []bool{true, false}) {
		t.Fatalf("not resumed: %v", events)
	}
	press(evdev.BTN_SOUTH)
	if last := r.log[len(r.log)-1]; last != "BTN_MOUSE/BTN_LEFT down" {
		t.Fatalf("after resume: %v", r.log)
	}
}

func TestComboLongestWins(t *testing.T) {
	cfg := mustParse(t, comboConfig)
	e, r, _ := newEngine()
	p := cfg.Default
	e.Handle(Button{evdev.BTN_TL, true}, p)
	e.Handle(Button{evdev.BTN_SOUTH, true}, p) // BTN_LEFT down: no combo yet
	e.Handle(Button{evdev.BTN_TR, true}, p)    // completes the 3-button combo
	e.Handle(Button{evdev.BTN_TR, false}, p)
	want := []string{
		"BTN_MOUSE/BTN_LEFT down",
		"KEY_LEFTCTRL down", "KEY_LEFTSHIFT down", "KEY_W down",
		"KEY_W up", "KEY_LEFTSHIFT up", "KEY_LEFTCTRL up",
	}
	if !slices.Equal(r.log, want) {
		t.Fatalf("got  %v\nwant %v", r.log, want)
	}
}

func TestDisconnect(t *testing.T) {
	cfg := mustParse(t, comboConfig)
	e, r, _ := newEngine()
	p := cfg.Default
	e.Handle(Button{evdev.BTN_TL, true}, p)
	e.Handle(Button{evdev.BTN_SOUTH, true}, p)
	e.Disconnect()
	if last := r.log[len(r.log)-1]; last != "BTN_MOUSE/BTN_LEFT up" {
		t.Fatalf("not released on disconnect: %v", r.log)
	}
	// Buttons held on the old pad are forgotten: TR alone is not a combo.
	r.log = nil
	e.Handle(Button{evdev.BTN_TR, true}, p)
	if len(r.log) != 0 {
		t.Fatalf("stale combo after reconnect: %v", r.log)
	}
}

func TestSharedKeyStaysDown(t *testing.T) {
	cfg := mustParse(t, `
[default]
BTN_TL = "ctrl"
BTN_TR = "ctrl+c"
`)
	e, r, _ := newEngine()
	e.Handle(Button{evdev.BTN_TL, true}, cfg.Default)
	e.Handle(Button{evdev.BTN_TR, true}, cfg.Default)
	e.Handle(Button{evdev.BTN_TR, false}, cfg.Default)
	if want := []string{"KEY_LEFTCTRL down", "KEY_C down", "KEY_C up"}; !slices.Equal(r.log, want) {
		t.Fatalf("got %v, want %v (Ctrl still held by BTN_TL)", r.log, want)
	}
	e.Handle(Button{evdev.BTN_TL, false}, cfg.Default)
	if got := r.log[len(r.log)-1]; got != "KEY_LEFTCTRL up" {
		t.Fatalf("last event %q, want Ctrl released", got)
	}
}

func TestAltTabKeepsAltWithAltChord(t *testing.T) {
	cfg := mustParse(t, `
[default]
BTN_TR    = "alt-tab"
BTN_SOUTH = "alt+f4"
`)
	e, r, _ := newEngine()
	e.Handle(Button{evdev.BTN_TR, true}, cfg.Default)
	e.Handle(Button{evdev.BTN_SOUTH, true}, cfg.Default)
	e.Handle(Button{evdev.BTN_SOUTH, false}, cfg.Default)
	if slices.Contains(r.log, "KEY_LEFTALT up") {
		t.Fatalf("Alt released while the switcher is open: %v", r.log)
	}
	e.Handle(Button{evdev.BTN_TR, false}, cfg.Default)
	if got := r.log[len(r.log)-1]; got != "KEY_LEFTALT up" {
		t.Fatalf("last event %q, want Alt released", got)
	}
}

func TestComboTieIsDeterministic(t *testing.T) {
	// BTN_SOUTH < BTN_EAST < BTN_NORTH: with all three held, both combos are
	// complete when BTN_EAST is pressed last, and the lowest buttons win.
	cfg := mustParse(t, `
[default]
"BTN_SOUTH+BTN_EAST" = "KEY_A"
"BTN_EAST+BTN_NORTH" = "KEY_B"
`)
	for range 50 { // map iteration order varies between runs
		e, r, _ := newEngine()
		e.Handle(Button{evdev.BTN_SOUTH, true}, cfg.Default)
		e.Handle(Button{evdev.BTN_NORTH, true}, cfg.Default)
		e.Handle(Button{evdev.BTN_EAST, true}, cfg.Default)
		if want := []string{"KEY_A down"}; !slices.Equal(r.log, want) {
			t.Fatalf("got %v, want %v", r.log, want)
		}
	}
}
