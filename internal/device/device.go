// SPDX-License-Identifier: GPL-3.0-or-later

// Package device finds and reads the physical gamepad.
package device

import (
	"errors"
	"fmt"
	"path/filepath"
	"slices"
	"strconv"
	"strings"

	"github.com/holoplot/go-evdev"
)

// ErrNotFound is returned when no gamepad is connected.
var ErrNotFound = errors.New("no gamepad found")

// ErrNotGamepad is returned when an explicitly given device is not a gamepad.
var ErrNotGamepad = errors.New("does not look like a gamepad")

// Gamepad is an opened physical gamepad.
type Gamepad struct {
	dev  *evdev.InputDevice
	Name string
	Path string
	// Abs holds the range of each absolute axis, used to normalise sticks and triggers.
	Abs map[evdev.EvCode]evdev.AbsInfo
}

// Open opens the gamepad at path, or the first detected one when path is empty.
func Open(path string) (*Gamepad, error) {
	if path != "" {
		dev, err := evdev.Open(path)
		if err != nil {
			return nil, err
		}
		if !isGamepad(dev) {
			dev.Close()
			return nil, fmt.Errorf("%s %w", path, ErrNotGamepad)
		}
		return wrap(dev)
	}

	paths, err := filepath.Glob("/dev/input/event*")
	if err != nil {
		return nil, err
	}
	// Numeric order (event2 before event10): lower numbers were usually
	// created first, so with several gamepads the first one plugged in wins.
	slices.SortFunc(paths, func(a, b string) int { return eventNumber(a) - eventNumber(b) })
	for _, p := range paths {
		dev, err := evdev.Open(p)
		if err != nil {
			continue
		}
		if isGamepad(dev) {
			return wrap(dev)
		}
		dev.Close()
	}
	return nil, ErrNotFound
}

func eventNumber(path string) int {
	n, err := strconv.Atoi(strings.TrimPrefix(filepath.Base(path), "event"))
	if err != nil {
		return -1
	}
	return n
}

// isGamepad detects a gamepad by capabilities rather than by name:
// a south face button and a left stick.
func isGamepad(dev *evdev.InputDevice) bool {
	return slices.Contains(dev.CapableEvents(evdev.EV_KEY), evdev.BTN_SOUTH) &&
		slices.Contains(dev.CapableEvents(evdev.EV_ABS), evdev.ABS_X) &&
		slices.Contains(dev.CapableEvents(evdev.EV_ABS), evdev.ABS_Y)
}

func wrap(dev *evdev.InputDevice) (*Gamepad, error) {
	name, _ := dev.Name()
	abs, err := dev.AbsInfos()
	if err != nil {
		dev.Close()
		return nil, fmt.Errorf("reading axis info: %w", err)
	}
	return &Gamepad{dev: dev, Name: name, Path: dev.Path(), Abs: abs}, nil
}

// Grab takes exclusive access so the desktop does not also see the raw events.
func (g *Gamepad) Grab() error { return g.dev.Grab() }

// Ungrab releases exclusive access.
func (g *Gamepad) Ungrab() error { return g.dev.Ungrab() }

// Close closes the device; a blocked Read returns an error.
func (g *Gamepad) Close() error { return g.dev.Close() }

// Read blocks until the next event. It returns an error when the device is
// unplugged or closed.
func (g *Gamepad) Read() (*evdev.InputEvent, error) { return g.dev.ReadOne() }
