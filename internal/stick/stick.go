// SPDX-License-Identifier: GPL-3.0-or-later

// Package stick turns raw analog stick values into cursor or scroll motion.
package stick

import (
	"math"

	"github.com/holoplot/go-evdev"
)

// Settings controls the response of a stick.
type Settings struct {
	Deadzone float64 // radial, as a fraction of full deflection
	Curve    float64 // response exponent; 1 = linear
}

// Normalize maps a raw axis value to [-1, 1] using the axis range.
func Normalize(v int32, info evdev.AbsInfo) float64 {
	half := (float64(info.Maximum) - float64(info.Minimum)) / 2
	if half <= 0 {
		return 0
	}
	center := float64(info.Minimum) + half
	return math.Max(-1, math.Min(1, (float64(v)-center)/half))
}

// Shape applies a radial dead zone and the response curve to a normalized
// (x, y) position. It returns (0, 0) inside the dead zone and keeps the
// direction of the stick otherwise.
func Shape(x, y float64, s Settings) (float64, float64) {
	m := math.Hypot(x, y)
	if m <= s.Deadzone || m == 0 {
		return 0, 0
	}
	r := (math.Min(m, 1) - s.Deadzone) / (1 - s.Deadzone)
	if s.Curve > 0 {
		r = math.Pow(r, s.Curve)
	}
	return x / m * r, y / m * r
}

// Active reports whether (x, y) lies outside the dead zone.
func Active(x, y float64, s Settings) bool {
	return math.Hypot(x, y) > s.Deadzone
}

// Accumulator keeps the sub-unit remainder of fractional motion so slow
// movements still add up instead of being rounded away.
type Accumulator struct {
	rx, ry float64
}

// Add accumulates (dx, dy) and returns the whole units ready to emit.
func (a *Accumulator) Add(dx, dy float64) (int32, int32) {
	a.rx += dx
	a.ry += dy
	ix, iy := math.Trunc(a.rx), math.Trunc(a.ry)
	a.rx -= ix
	a.ry -= iy
	return int32(ix), int32(iy)
}

// Reset drops any pending remainder.
func (a *Accumulator) Reset() { a.rx, a.ry = 0, 0 }

// Stick tracks the position of one analog stick and converts it to motion.
type Stick struct {
	xCode, yCode evdev.EvCode
	xInfo, yInfo evdev.AbsInfo
	x, y         float64
	settings     Settings
	acc          Accumulator
}

// New tracks the stick made of axes xCode and yCode, whose ranges and
// current positions are read from abs.
func New(xCode, yCode evdev.EvCode, abs map[evdev.EvCode]evdev.AbsInfo, s Settings) *Stick {
	st := &Stick{xCode: xCode, yCode: yCode, xInfo: abs[xCode], yInfo: abs[yCode], settings: s}
	st.x = Normalize(st.xInfo.Value, st.xInfo)
	st.y = Normalize(st.yInfo.Value, st.yInfo)
	return st
}

// Update records an axis event. It returns false if the axis is not part of this stick.
func (s *Stick) Update(code evdev.EvCode, value int32) bool {
	switch code {
	case s.xCode:
		s.x = Normalize(value, s.xInfo)
	case s.yCode:
		s.y = Normalize(value, s.yInfo)
	default:
		return false
	}
	return true
}

// Active reports whether the stick is outside its dead zone.
func (s *Stick) Active() bool { return Active(s.x, s.y, s.settings) }

// Step returns the whole units of motion produced over dt seconds at the
// current position, speed being the units per second at full deflection.
// Inside the dead zone it returns (0, 0) and drops any remainder, so a
// stale fraction does not leak into the next movement.
func (s *Stick) Step(dt, speed float64) (int32, int32) {
	if !s.Active() {
		s.acc.Reset()
		return 0, 0
	}
	x, y := Shape(s.x, s.y, s.settings)
	k := speed * dt
	return s.acc.Add(x*k, y*k)
}
