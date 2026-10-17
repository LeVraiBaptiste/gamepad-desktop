// SPDX-License-Identifier: GPL-3.0-or-later

package main

import (
	"context"
	"log"
	"time"

	"github.com/LeVraiBaptiste/gamepad-desktop/internal/config"
	"github.com/LeVraiBaptiste/gamepad-desktop/internal/focus"
)

// focusTimeout bounds the focus query so a stuck compositor cannot freeze
// the buttons; the default profile is used instead.
const focusTimeout = 100 * time.Millisecond

// resolver picks the profile of the focused window. It is called on button
// presses and when a stick starts moving, never in the background.
type resolver struct {
	cfg      *config.Config
	provider focus.Provider // nil when no provider could be created
	verbose  bool
	failing  bool // a warning was logged and focus has not worked since
}

func (r *resolver) profile() *config.Profile {
	if r.provider == nil || len(r.cfg.Apps) == 0 {
		return r.cfg.Default
	}
	ctx, cancel := context.WithTimeout(context.Background(), focusTimeout)
	defer cancel()
	w, ok, err := r.provider.Focused(ctx)
	if err != nil {
		if !r.failing {
			log.Printf("focus: %v; using [default] until it works", err)
			r.failing = true
		}
		return r.cfg.Default
	}
	if r.failing {
		log.Printf("focus: working again")
		r.failing = false
	}
	if !ok {
		if r.verbose {
			log.Printf("  focus: no focused window")
		}
		return r.cfg.Default
	}
	p := r.cfg.Profile(w.WMClass, w.Instance)
	if r.verbose {
		log.Printf("  focus: wm_class=%q instance=%q → [%s]", w.WMClass, w.Instance, p.Name)
	}
	return p
}
