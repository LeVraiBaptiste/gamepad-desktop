// SPDX-License-Identifier: GPL-3.0-or-later

package config

import (
	"os"
	"slices"
	"strings"
	"testing"

	"github.com/holoplot/go-evdev"
)

func TestParseExample(t *testing.T) {
	data, err := os.ReadFile("../../examples/config.toml")
	if err != nil {
		t.Fatal(err)
	}
	cfg, err := Parse(string(data), "examples/config.toml")
	if err != nil {
		t.Fatal(err)
	}

	zen := cfg.Profile("Zen")
	if zen.Name != "zen" {
		t.Fatalf("Profile(Zen) = %q, want zen (case-insensitive)", zen.Name)
	}
	if got := zen.Buttons[evdev.BTN_NORTH].Steps; !slices.Equal(got[0], []evdev.EvCode{evdev.KEY_LEFTCTRL, evdev.KEY_T}) {
		t.Errorf("zen BTN_NORTH = %v", got)
	}
	// Inherited from default.
	if got := zen.Buttons[evdev.BTN_SOUTH].Steps; got[0][0] != evdev.BTN_LEFT {
		t.Errorf("zen BTN_SOUTH not inherited: %v", got)
	}
	if zen.Sticks[RStick].Kind != Cursor || zen.Sticks[LStick].Kind != Scroll {
		t.Errorf("zen sticks not inherited: %+v", zen.Sticks)
	}
	// Not leaked into default.
	if _, ok := cfg.Default.Buttons[evdev.BTN_NORTH]; ok {
		t.Error("app mapping leaked into default")
	}

	seq := cfg.Profile("kitty").Buttons[evdev.BTN_DPAD_UP]
	if len(seq.Steps) != 2 || seq.Steps[1][0] != evdev.KEY_K {
		t.Errorf("kitty DPAD_UP sequence = %v", seq.Steps)
	}
	if cfg.Profile("unknown-class", "kitty") != cfg.Profile("kitty") {
		t.Error("should fall back to the second name")
	}
	if cfg.Profile("firefox") != cfg.Default {
		t.Error("unknown app should get the default profile")
	}
	if cfg.Settings.TriggerThreshold != 0.5 {
		t.Errorf("unset setting should keep its default, got %v", cfg.Settings.TriggerThreshold)
	}
}

func TestBuiltin(t *testing.T) {
	if _, err := Parse(builtin, ""); err != nil {
		t.Fatal(err)
	}
}

func TestNoneUnbinds(t *testing.T) {
	cfg, err := Parse(`
[default]
BTN_SOUTH = "BTN_LEFT"
[app.game]
BTN_SOUTH = "none"
`, "")
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := cfg.Profile("game").Buttons[evdev.BTN_SOUTH]; ok {
		t.Error(`"none" should remove the inherited mapping`)
	}
}

func TestParseAction(t *testing.T) {
	tests := []struct {
		in   string
		want [][]evdev.EvCode
	}{
		{"KEY_A", [][]evdev.EvCode{{evdev.KEY_A}}},
		{"ctrl+shift+t", [][]evdev.EvCode{{evdev.KEY_LEFTCTRL, evdev.KEY_LEFTSHIFT, evdev.KEY_T}}},
		{"KEY_LEFTCTRL+KEY_B, KEY_K", [][]evdev.EvCode{{evdev.KEY_LEFTCTRL, evdev.KEY_B}, {evdev.KEY_K}}},
		{"pageup", [][]evdev.EvCode{{evdev.KEY_PAGEUP}}},
		{"BTN_MIDDLE", [][]evdev.EvCode{{evdev.BTN_MIDDLE}}},
	}
	for _, tt := range tests {
		a, err := ParseAction(tt.in)
		if err != nil {
			t.Errorf("ParseAction(%q): %v", tt.in, err)
			continue
		}
		if !slices.EqualFunc(a.Steps, tt.want, slices.Equal) {
			t.Errorf("ParseAction(%q) = %v, want %v", tt.in, a.Steps, tt.want)
		}
	}
	if a, _ := ParseAction("Alt-Tab"); a.Kind != AltTab {
		t.Errorf("alt-tab kind = %v", a.Kind)
	}
}

func TestErrors(t *testing.T) {
	_, err := Parse(`
[settings]
deadzone = 1.5
speed = 3

[default]
BTN_SOUTH = "KEY_LEFTCRTL+KEY_T"
BTN_FOO   = "KEY_A"
BTN_EAST  = "cursor"
RSTICK    = "KEY_A"
BTN_WEST  = "KEY_A+"
BTN_NORTH = "BTN_SOUTH"
`, "test.toml")
	if err == nil {
		t.Fatal("expected errors")
	}
	msg := err.Error()
	for _, want := range []string{
		"test.toml:",
		`unknown setting "settings.speed"`,
		"deadzone must be in [0, 1)",
		`[default] BTN_SOUTH = "KEY_LEFTCRTL+KEY_T": unknown key "KEY_LEFTCRTL"`,
		`unknown gamepad input "BTN_FOO"`,
		`"cursor" only applies to LSTICK or RSTICK`,
		`a stick can only be "cursor", "scroll" or "none"`,
		"empty key name",
		`"BTN_SOUTH" cannot be emitted`,
	} {
		if !strings.Contains(msg, want) {
			t.Errorf("error does not mention %q:\n%s", want, msg)
		}
	}
}

func TestSyntaxError(t *testing.T) {
	_, err := Parse("[default\nBTN_SOUTH = 1", "bad.toml")
	if err == nil || !strings.Contains(err.Error(), "bad.toml") {
		t.Fatalf("got %v", err)
	}
}

func TestCombos(t *testing.T) {
	cfg, err := Parse(`
[default]
"BTN_START+BTN_SELECT" = "pause"
[app.game]
"btn_select + btn_start" = "none"
`, "")
	if err != nil {
		t.Fatal(err)
	}
	c, ok := cfg.Default.Combos[comboKey([]evdev.EvCode{evdev.BTN_SELECT, evdev.BTN_START})]
	if !ok || c.Action.Kind != Pause {
		t.Fatalf("combo not parsed (order-independent): %+v", cfg.Default.Combos)
	}
	if len(cfg.Profile("game").Combos) != 0 {
		t.Error(`"none" should remove the inherited combo`)
	}

	_, err = Parse(`
[default]
"BTN_START+BTN_START" = "pause"
"LSTICK+BTN_START" = "pause"
`, "")
	for _, want := range []string{"appears twice", "sticks cannot be part of a combo"} {
		if err == nil || !strings.Contains(err.Error(), want) {
			t.Errorf("error should mention %q: %v", want, err)
		}
	}
}

func TestDuplicateAppCase(t *testing.T) {
	_, err := Parse(`
[app.Zen]
BTN_SOUTH = "KEY_A"
[app.zen]
BTN_SOUTH = "KEY_B"
`, "")
	if err == nil || !strings.Contains(err.Error(), "duplicates") {
		t.Fatalf("got %v, want a duplicate error", err)
	}
}
