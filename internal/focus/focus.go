// SPDX-License-Identifier: GPL-3.0-or-later

// Package focus finds the window that currently has the keyboard focus.
package focus

import (
	"context"
	"errors"
)

// Window describes the focused window.
type Window struct {
	WMClass  string // e.g. "zen", "kitty", "org.gnome.Nautilus"
	Instance string // WM_CLASS instance, sometimes more specific than the class
}

// ErrUnavailable means the provider cannot work in this session (missing
// compositor support or extension). The caller should fall back to the
// default profile.
var ErrUnavailable = errors.New("focus provider unavailable")

// Provider reports the focused window. Implementations exist per compositor.
type Provider interface {
	// Focused returns the focused window, or ok=false when no window has
	// the focus (e.g. the desktop or the overview).
	Focused(ctx context.Context) (w Window, ok bool, err error)
	Close() error
}
