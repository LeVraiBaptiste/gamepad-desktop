// SPDX-License-Identifier: GPL-3.0-or-later

// Package config loads the TOML configuration: global settings, a default
// profile and per-application profiles that inherit from it.
package config

import (
	_ "embed"
	"errors"
	"fmt"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/BurntSushi/toml"
	"github.com/holoplot/go-evdev"

	"github.com/LeVraiBaptiste/gamepad-desktop/internal/output"
)

//go:embed default.toml
var builtin string

// Settings are the global tuning knobs.
type Settings struct {
	Deadzone         float64 `toml:"deadzone"`          // stick dead zone, fraction of full deflection
	Sensitivity      float64 `toml:"sensitivity"`       // cursor speed, px/s at full deflection
	Curve            float64 `toml:"curve"`             // stick response exponent, 1 = linear
	ScrollSpeed      float64 `toml:"scroll_speed"`      // wheel notches/s at full deflection
	TriggerThreshold float64 `toml:"trigger_threshold"` // fraction of travel at which a trigger presses
	SequenceDelayMS  int     `toml:"sequence_delay_ms"` // pause between the steps of a sequence
}

// SequenceDelay returns SequenceDelayMS as a duration.
func (s Settings) SequenceDelay() time.Duration {
	return time.Duration(s.SequenceDelayMS) * time.Millisecond
}

var defaultSettings = Settings{
	Deadzone:         0.15,
	Sensitivity:      1200,
	Curve:            2,
	ScrollSpeed:      15,
	TriggerThreshold: 0.5,
	SequenceDelayMS:  15,
}

// Kind is the type of an action.
type Kind int

const (
	None   Kind = iota // explicitly unbound
	Keys               // key chords, one or several steps
	Cursor             // stick moves the pointer
	Scroll             // stick scrolls
	AltTab             // stateful window switcher
	Pause              // toggles the daemon off and on
)

// Action is what an input does.
type Action struct {
	Kind Kind
	// Steps holds the chords for Keys: one step is a held combination,
	// several steps are tapped in sequence.
	Steps [][]evdev.EvCode
	// Source is the action as written in the config, for logs.
	Source string
}

// Stick identifies an analog stick.
type Stick int

const (
	LStick Stick = iota
	RStick
)

// Profile is a complete set of mappings.
type Profile struct {
	Name    string
	Buttons map[evdev.EvCode]Action
	Sticks  [2]Action
	// Combos are actions bound to several buttons pressed together,
	// keyed by their sorted button list (see comboKey).
	Combos map[string]Combo
}

// Combo is an action triggered by holding several buttons at once. It fires
// when the last of them is pressed.
type Combo struct {
	Buttons []evdev.EvCode // sorted
	Action  Action
	Input   string // as written in the config, for logs
}

func comboKey(buttons []evdev.EvCode) string {
	return fmt.Sprint(buttons)
}

// Config is a validated configuration.
type Config struct {
	Path     string // file it was loaded from, "" for the built-in one
	Settings Settings
	Default  *Profile
	Apps     map[string]*Profile // keyed by lower-case wm_class
}

// Profile returns the profile of the first name that has one (names are
// matched case-insensitively), or the default profile.
func (c *Config) Profile(names ...string) *Profile {
	for _, n := range names {
		if n == "" {
			continue
		}
		if p, ok := c.Apps[strings.ToLower(n)]; ok {
			return p
		}
	}
	return c.Default
}

type rawConfig struct {
	Settings Settings                     `toml:"settings"`
	Default  map[string]string            `toml:"default"`
	App      map[string]map[string]string `toml:"app"`
}

// Paths returns the locations searched for the config file, in order.
func Paths() []string {
	var paths []string
	if dir := os.Getenv("XDG_CONFIG_HOME"); dir != "" {
		paths = append(paths, filepath.Join(dir, "gamepad-desktop", "config.toml"))
	} else if home, err := os.UserHomeDir(); err == nil {
		paths = append(paths, filepath.Join(home, ".config", "gamepad-desktop", "config.toml"))
	}
	return append(paths, "/etc/gamepad-desktop/config.toml")
}

// Load reads the config at path. With an empty path it tries Paths() in
// order and falls back to the built-in configuration.
func Load(path string) (*Config, error) {
	if path != "" {
		return loadFile(path)
	}
	for _, p := range Paths() {
		c, err := loadFile(p)
		if errors.Is(err, os.ErrNotExist) {
			continue
		}
		return c, err
	}
	return Parse(builtin, "")
}

func loadFile(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	return Parse(string(data), path)
}

// Parse parses and validates a configuration. path is only used in errors.
func Parse(data, path string) (*Config, error) {
	name := path
	if name == "" {
		name = "built-in config"
	}

	raw := rawConfig{Settings: defaultSettings}
	md, err := toml.Decode(data, &raw)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", name, err)
	}

	var errs []error
	for _, k := range md.Undecoded() {
		errs = append(errs, fmt.Errorf("unknown setting %q", k.String()))
	}
	errs = append(errs, validateSettings(raw.Settings)...)

	cfg := &Config{Path: path, Settings: raw.Settings, Apps: map[string]*Profile{}}
	cfg.Default = &Profile{Name: "default", Buttons: map[evdev.EvCode]Action{}, Combos: map[string]Combo{}}
	errs = append(errs, apply(cfg.Default, "default", raw.Default)...)

	for _, app := range slices.Sorted(maps.Keys(raw.App)) {
		if other, ok := cfg.Apps[strings.ToLower(app)]; ok {
			errs = append(errs, fmt.Errorf("[app.%s] duplicates [app.%s]: wm_class is matched case-insensitively", app, other.Name))
			continue
		}
		p := &Profile{Name: app, Buttons: maps.Clone(cfg.Default.Buttons), Sticks: cfg.Default.Sticks, Combos: maps.Clone(cfg.Default.Combos)}
		errs = append(errs, apply(p, "app."+app, raw.App[app])...)
		cfg.Apps[strings.ToLower(app)] = p
	}

	if len(errs) > 0 {
		return nil, fmt.Errorf("%s:\n  %w", name, indented(errs))
	}
	return cfg, nil
}

func validateSettings(s Settings) []error {
	var errs []error
	check := func(ok bool, format string, args ...any) {
		if !ok {
			errs = append(errs, fmt.Errorf("[settings] "+format, args...))
		}
	}
	check(s.Deadzone >= 0 && s.Deadzone < 1, "deadzone must be in [0, 1), got %v", s.Deadzone)
	check(s.Sensitivity > 0, "sensitivity must be positive, got %v", s.Sensitivity)
	check(s.Curve > 0, "curve must be positive, got %v", s.Curve)
	check(s.ScrollSpeed > 0, "scroll_speed must be positive, got %v", s.ScrollSpeed)
	check(s.TriggerThreshold > 0.1 && s.TriggerThreshold <= 1, "trigger_threshold must be in (0.1, 1], got %v", s.TriggerThreshold)
	check(s.SequenceDelayMS >= 0 && s.SequenceDelayMS <= 1000, "sequence_delay_ms must be in [0, 1000], got %v", s.SequenceDelayMS)
	return errs
}

// apply overlays the mappings of one TOML section onto p.
func apply(p *Profile, section string, raw map[string]string) []error {
	var errs []error
	for _, input := range slices.Sorted(maps.Keys(raw)) {
		value := raw[input]
		fail := func(err error) {
			errs = append(errs, fmt.Errorf("[%s] %s = %q: %w", section, input, value, err))
		}

		if stick, ok := parseStick(input); ok {
			a, err := ParseAction(value)
			if err != nil || (a.Kind != Cursor && a.Kind != Scroll && a.Kind != None) {
				fail(errors.New(`a stick can only be "cursor", "scroll" or "none"`))
				continue
			}
			p.Sticks[stick] = a
			continue
		}

		codes, err := parseInputs(input)
		if err != nil {
			fail(err)
			continue
		}
		a, err := ParseAction(value)
		if err == nil && (a.Kind == Cursor || a.Kind == Scroll) {
			err = fmt.Errorf("%q only applies to LSTICK or RSTICK", value)
		}
		if err != nil {
			fail(err)
			continue
		}
		if len(codes) > 1 {
			key := comboKey(codes)
			if a.Kind == None {
				delete(p.Combos, key)
			} else {
				p.Combos[key] = Combo{Buttons: codes, Action: a, Input: input}
			}
			continue
		}
		if a.Kind == None {
			delete(p.Buttons, codes[0])
		} else {
			p.Buttons[codes[0]] = a
		}
	}
	return errs
}

func parseStick(name string) (Stick, bool) {
	switch strings.ToUpper(strings.TrimSpace(name)) {
	case "LSTICK":
		return LStick, true
	case "RSTICK":
		return RStick, true
	}
	return 0, false
}

var dpadNames = map[string]evdev.EvCode{
	"DPAD_UP":    evdev.BTN_DPAD_UP,
	"DPAD_DOWN":  evdev.BTN_DPAD_DOWN,
	"DPAD_LEFT":  evdev.BTN_DPAD_LEFT,
	"DPAD_RIGHT": evdev.BTN_DPAD_RIGHT,
}

// IsGamepadButton reports whether code is a gamepad button or a d-pad direction.
func IsGamepadButton(code evdev.EvCode) bool {
	return (code >= evdev.BTN_GAMEPAD && code <= evdev.BTN_THUMBR) ||
		(code >= evdev.BTN_DPAD_UP && code <= evdev.BTN_DPAD_RIGHT)
}

// parseInputs parses one input or a combo of distinct inputs joined by "+",
// such as "BTN_SELECT+BTN_START". The result is sorted.
func parseInputs(name string) ([]evdev.EvCode, error) {
	var codes []evdev.EvCode
	for part := range strings.SplitSeq(name, "+") {
		if _, ok := parseStick(part); ok {
			return nil, errors.New("sticks cannot be part of a combo")
		}
		code, err := ParseInput(part)
		if err != nil {
			return nil, err
		}
		if slices.Contains(codes, code) {
			return nil, fmt.Errorf("%s appears twice", strings.TrimSpace(part))
		}
		codes = append(codes, code)
	}
	slices.Sort(codes)
	return codes, nil
}

// ParseInput resolves a gamepad input name such as BTN_SOUTH, BTN_A or DPAD_UP.
func ParseInput(name string) (evdev.EvCode, error) {
	up := strings.ToUpper(strings.TrimSpace(name))
	if code, ok := dpadNames[up]; ok {
		return code, nil
	}
	if code, ok := evdev.KEYFromString[up]; ok && IsGamepadButton(code) {
		return code, nil
	}
	return 0, fmt.Errorf("unknown gamepad input %q (expected e.g. BTN_SOUTH, BTN_TL2, DPAD_UP, LSTICK)", name)
}

var keyAliases = map[string]string{
	"CTRL":  "KEY_LEFTCTRL",
	"SHIFT": "KEY_LEFTSHIFT",
	"ALT":   "KEY_LEFTALT",
	"SUPER": "KEY_LEFTMETA",
	"META":  "KEY_LEFTMETA",
}

// ParseKey resolves an output key name. Names are case-insensitive, the
// KEY_ prefix is optional, and CTRL/SHIFT/ALT/SUPER are accepted.
func ParseKey(name string) (evdev.EvCode, error) {
	up := strings.ToUpper(strings.TrimSpace(name))
	if alias, ok := keyAliases[up]; ok {
		up = alias
	}
	code, ok := evdev.KEYFromString[up]
	if !ok && !strings.HasPrefix(up, "KEY_") && !strings.HasPrefix(up, "BTN_") {
		code, ok = evdev.KEYFromString["KEY_"+up]
	}
	if !ok {
		return 0, fmt.Errorf("unknown key %q", name)
	}
	if !output.Supports(code) {
		return 0, fmt.Errorf("%q cannot be emitted (only keyboard keys and BTN_LEFT/RIGHT/MIDDLE/SIDE/EXTRA)", name)
	}
	return code, nil
}

// ParseAction parses an action: a special action name, or key chords
// joined by "+" and sequenced by ",".
func ParseAction(s string) (Action, error) {
	src := strings.TrimSpace(s)
	switch strings.ToLower(src) {
	case "":
		return Action{}, errors.New("empty action")
	case "none":
		return Action{Kind: None, Source: src}, nil
	case "cursor":
		return Action{Kind: Cursor, Source: src}, nil
	case "scroll":
		return Action{Kind: Scroll, Source: src}, nil
	case "alt-tab":
		return Action{Kind: AltTab, Source: src}, nil
	case "pause":
		return Action{Kind: Pause, Source: src}, nil
	}

	a := Action{Kind: Keys, Source: src}
	for step := range strings.SplitSeq(src, ",") {
		var chord []evdev.EvCode
		for key := range strings.SplitSeq(step, "+") {
			if strings.TrimSpace(key) == "" {
				return Action{}, errors.New(`empty key name (check for a stray "+" or ",")`)
			}
			code, err := ParseKey(key)
			if err != nil {
				return Action{}, err
			}
			chord = append(chord, code)
		}
		a.Steps = append(a.Steps, chord)
	}
	return a, nil
}

type indented []error

func (e indented) Error() string {
	parts := make([]string, len(e))
	for i, err := range e {
		parts[i] = err.Error()
	}
	return strings.Join(parts, "\n  ")
}

func (e indented) Unwrap() []error { return e }
