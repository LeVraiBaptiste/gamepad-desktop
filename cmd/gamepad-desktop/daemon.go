// SPDX-License-Identifier: GPL-3.0-or-later

package main

import (
	"errors"
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/holoplot/go-evdev"

	"github.com/LeVraiBaptiste/gamepad-desktop/internal/config"
	"github.com/LeVraiBaptiste/gamepad-desktop/internal/device"
	"github.com/LeVraiBaptiste/gamepad-desktop/internal/focus"
	"github.com/LeVraiBaptiste/gamepad-desktop/internal/mapper"
	"github.com/LeVraiBaptiste/gamepad-desktop/internal/output"
	"github.com/LeVraiBaptiste/gamepad-desktop/internal/stick"
)

const (
	// tickInterval is the motion update period while a stick is deflected.
	tickInterval = 8 * time.Millisecond
	// settleDelay lets a burst of /dev/input changes finish (node creation,
	// then udev permissions) before trying to open the gamepad.
	settleDelay = 200 * time.Millisecond
	// pollInterval is the fallback when /dev/input cannot be watched.
	pollInterval = 5 * time.Second
)

type daemon struct {
	cfg     *config.Config
	devPath string
	verbose bool
	sig     chan os.Signal

	out    *output.Output
	engine *mapper.Engine
	res    *resolver
	pad    *device.Gamepad // nil while waiting for a gamepad
}

// errStop ends the daemon cleanly, on SIGINT or SIGTERM.
var errStop = errors.New("stopped")

func run(cfg *config.Config, devPath string, verbose bool) error {
	out, err := output.New()
	if err != nil {
		return err
	}
	defer out.Close()

	d := &daemon{cfg: cfg, devPath: devPath, verbose: verbose, sig: make(chan os.Signal, 1), out: out}
	signal.Notify(d.sig, syscall.SIGINT, syscall.SIGTERM)

	d.engine = mapper.NewEngine(out, cfg.Settings.SequenceDelay())
	d.engine.OnPause = d.onPause
	if verbose {
		d.engine.Logf = log.Printf
	}
	defer d.engine.ReleaseAll()

	d.res = &resolver{cfg: cfg, verbose: verbose}
	if len(cfg.Apps) > 0 {
		if g, err := focus.NewGNOME(); err != nil {
			log.Printf("focus: %v; app profiles disabled", err)
		} else {
			d.res.provider = g
			defer g.Close()
		}
	}

	for {
		pad, err := d.waitPad()
		if errors.Is(err, errStop) {
			return nil
		} else if err != nil {
			return err
		}
		d.pad = pad
		err = d.serve(pad)
		pad.Close()
		d.pad = nil
		if err := d.engine.Disconnect(); err != nil {
			log.Printf("output: %v", err)
		}
		if errors.Is(err, errStop) {
			return nil
		}
		log.Printf("gamepad disconnected (%v); waiting for it to come back", err)
	}
}

func (d *daemon) stop(s os.Signal) error {
	log.Printf("received %s, exiting", s)
	return errStop
}

// waitPad returns the gamepad, grabbed unless paused, waiting for one to be
// plugged in if needed. It does not wake up while nothing changes in /dev/input.
func (d *daemon) waitPad() (*device.Gamepad, error) {
	// Watch before the first attempt so a gamepad plugged in between the
	// attempt and the wait is not missed.
	w, err := device.Watch()
	if err != nil {
		log.Printf("cannot watch /dev/input (%v); checking every %v", err, pollInterval)
	} else {
		defer w.Close()
	}

	announced := false
	for {
		// grabbed is false when another program holds the gamepad. Releasing
		// it changes nothing in /dev/input, so that case is polled.
		grabbed := true
		pad, err := device.Open(d.devPath)
		switch {
		case err == nil:
			if !d.engine.Paused() {
				if err := pad.Grab(); err != nil {
					pad.Close()
					if !announced {
						log.Printf("gamepad %s: grab failed (%v); is another program using it? Retrying every %v",
							pad.Path, err, pollInterval)
					}
					grabbed = false
					break
				}
			}
			log.Printf("gamepad: %s (%s)", pad.Name, pad.Path)
			return pad, nil
		case errors.Is(err, device.ErrNotGamepad):
			return nil, err
		case !announced:
			log.Printf("waiting for a gamepad (%v)", err)
		}
		announced = true

		var changed <-chan struct{}
		var poll <-chan time.Time
		if w != nil {
			changed = w.C
		}
		if w == nil || !grabbed {
			poll = time.After(pollInterval)
		}
		select {
		case _, ok := <-changed:
			if !ok {
				w = nil // watcher failed: fall back to polling
			}
			time.Sleep(settleDelay)
		case <-poll:
		case s := <-d.sig:
			return nil, d.stop(s)
		}
	}
}

func (d *daemon) onPause(paused bool) {
	if d.pad == nil {
		return
	}
	if paused {
		if err := d.pad.Ungrab(); err != nil {
			log.Printf("pause: releasing the gamepad: %v", err)
		}
		log.Printf("paused: the gamepad is left to other applications until the pause action is used again")
	} else {
		if err := d.pad.Grab(); err != nil {
			log.Printf("resume: grabbing the gamepad: %v", err)
		}
		log.Printf("resumed")
	}
}

// serve handles one connected gamepad until it goes away (returning the read
// error) or a signal arrives (returning errStop).
func (d *daemon) serve(pad *device.Gamepad) error {
	events := make(chan *evdev.InputEvent)
	readErr := make(chan error, 1)
	done := make(chan struct{})
	defer close(done)
	go func() {
		for {
			ev, err := pad.Read()
			if err != nil {
				readErr <- err
				return
			}
			select {
			case events <- ev:
			case <-done:
				return
			}
		}
	}()

	set := d.cfg.Settings
	inputs := mapper.NewInputs(pad.Abs, set.TriggerThreshold)
	shape := stick.Settings{Deadzone: set.Deadzone, Curve: set.Curve}
	sticks := [2]*stick.Stick{
		config.LStick: stick.New(evdev.ABS_X, evdev.ABS_Y, pad.Abs, shape),
		config.RStick: stick.New(evdev.ABS_RX, evdev.ABS_RY, pad.Abs, shape),
	}

	// The ticker only runs while a mapped stick is outside its dead zone,
	// so the daemon does not wake up at all when the gamepad is idle. The
	// profile is resolved when motion starts and kept until it stops.
	var (
		ticker          *time.Ticker
		tick            <-chan time.Time
		lastTick, start time.Time
		motion          *config.Profile
		moved, scrolled [2]int32 // summarised in verbose mode
		// checked records which sticks were active when the profile last
		// said none of them is mapped, so a deflected unmapped stick does
		// not query the focus on each of its events.
		checked [2]bool
	)
	defer func() {
		if ticker != nil {
			ticker.Stop()
		}
	}()
	moving := func(p *config.Profile) bool {
		for i, s := range sticks {
			if p.Sticks[i].Kind != config.None && s.Active() {
				return true
			}
		}
		return false
	}
	active := func() [2]bool { return [2]bool{sticks[0].Active(), sticks[1].Active()} }
	startTicker := func() {
		// Cheap checks first: focus is only queried when a stick leaves its
		// dead zone, not on every jitter of an idle stick, nor on every event
		// of a stick already found unmapped.
		if ticker != nil || d.engine.Paused() {
			return
		}
		a := active()
		newlyActive := (a[0] && !checked[0]) || (a[1] && !checked[1])
		checked = [2]bool{a[0] && checked[0], a[1] && checked[1]}
		if !newlyActive {
			return
		}
		if p := d.res.profile(); moving(p) {
			motion = p
			ticker = time.NewTicker(tickInterval)
			tick = ticker.C
			lastTick, start = time.Now(), time.Now()
			moved, scrolled = [2]int32{}, [2]int32{}
		} else {
			checked = a
		}
	}
	stopTicker := func(now time.Time) {
		ticker.Stop()
		ticker, tick = nil, nil
		// A stick still deflected here is unmapped in the motion profile.
		checked = active()
		if d.verbose {
			log.Printf("  → [%s] motion %.2fs: cursor x=%d y=%d px, scroll v=%d h=%d hi-res",
				motion.Name, now.Sub(start).Seconds(), moved[0], moved[1], scrolled[0], scrolled[1])
		}
	}
	startTicker()

	for {
		select {
		case ev := <-events:
			if ev.Type == evdev.EV_SYN {
				continue
			}
			if ev.Type == evdev.EV_ABS && (sticks[0].Update(ev.Code, ev.Value) || sticks[1].Update(ev.Code, ev.Value)) {
				startTicker()
				continue
			}
			for _, b := range inputs.Translate(ev) {
				if d.verbose {
					state := "up"
					if b.Down {
						state = "down"
					}
					log.Printf("%s %s", evdev.CodeName(evdev.EV_KEY, b.Code), state)
				}
				// Focus is only queried on press, and not while paused; a
				// release undoes what its press did, whatever the focus is now.
				var p *config.Profile
				if b.Down && !d.engine.Paused() {
					p = d.res.profile()
				}
				if err := d.engine.Handle(b, p); err != nil {
					log.Printf("output: %v", err)
				}
			}

		case now := <-tick:
			if d.engine.Paused() {
				stopTicker(now)
				checked = [2]bool{} // check again after resuming
				continue
			}
			dt := now.Sub(lastTick).Seconds()
			lastTick = now
			for i, s := range sticks {
				switch motion.Sticks[i].Kind {
				case config.Cursor:
					if dx, dy := s.Step(dt, set.Sensitivity); dx != 0 || dy != 0 {
						if err := d.out.Move(dx, dy); err != nil {
							log.Printf("output: %v", err)
						}
						moved[0] += dx
						moved[1] += dy
					}
				case config.Scroll:
					// Stick up (negative y) scrolls up (positive wheel).
					if h, v := s.Step(dt, set.ScrollSpeed*output.HiResPerNotch); h != 0 || v != 0 {
						if err := d.out.Scroll(-v, h); err != nil {
							log.Printf("output: %v", err)
						}
						scrolled[0] += -v
						scrolled[1] += h
					}
				}
			}
			if !moving(motion) {
				stopTicker(now)
			}

		case err := <-readErr:
			return fmt.Errorf("reading gamepad: %w", err)
		case s := <-d.sig:
			return d.stop(s)
		}
	}
}
