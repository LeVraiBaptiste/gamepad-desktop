// SPDX-License-Identifier: GPL-3.0-or-later

package stick

import (
	"math"
	"testing"

	"github.com/holoplot/go-evdev"
)

func near(a, b float64) bool { return math.Abs(a-b) < 1e-9 }

func TestNormalize(t *testing.T) {
	xpad := evdev.AbsInfo{Minimum: -32768, Maximum: 32767}
	trigger := evdev.AbsInfo{Minimum: 0, Maximum: 255}
	tests := []struct {
		v    int32
		info evdev.AbsInfo
		want float64
	}{
		{-32768, xpad, -1},
		{32767, xpad, 1},
		{0, trigger, -1},
		{255, trigger, 1},
		{999999, xpad, 1},
		{5, evdev.AbsInfo{}, 0},
	}
	for _, tt := range tests {
		if got := Normalize(tt.v, tt.info); !near(got, tt.want) {
			t.Errorf("Normalize(%d, %+v) = %v, want %v", tt.v, tt.info, got, tt.want)
		}
	}
	if got := Normalize(0, xpad); math.Abs(got) > 1e-4 {
		t.Errorf("Normalize(center) = %v, want ~0", got)
	}
}

func TestShape(t *testing.T) {
	s := Settings{Deadzone: 0.2, Curve: 2}

	if x, y := Shape(0.1, 0.1, s); x != 0 || y != 0 {
		t.Errorf("inside dead zone: got (%v, %v), want (0, 0)", x, y)
	}
	if x, y := Shape(1, 0, s); !near(x, 1) || !near(y, 0) {
		t.Errorf("full right: got (%v, %v), want (1, 0)", x, y)
	}
	// Halfway between dead zone and edge, squared.
	if x, _ := Shape(0.6, 0, s); !near(x, 0.25) {
		t.Errorf("half deflection: got %v, want 0.25", x)
	}
	// Corners overshoot the unit circle on some pads; magnitude is clamped.
	x, y := Shape(1, 1, s)
	if !near(math.Hypot(x, y), 1) || !near(x, y) {
		t.Errorf("corner: got (%v, %v), want magnitude 1 at 45°", x, y)
	}
	if x, _ := Shape(-0.6, 0, Settings{Deadzone: 0.2, Curve: 1}); !near(x, -0.5) {
		t.Errorf("linear left: got %v, want -0.5", x)
	}
}

func TestAccumulator(t *testing.T) {
	var a Accumulator
	var sum int32
	for range 12 {
		x, _ := a.Add(0.25, 0)
		sum += x
	}
	if sum != 3 {
		t.Errorf("12 × 0.25 emitted %d, want 3", sum)
	}
	if _, y := a.Add(0, -1.5); y != -1 {
		t.Errorf("negative: got %d, want -1", y)
	}
	if _, y := a.Add(0, -0.5); y != -1 {
		t.Errorf("negative remainder: got %d, want -1", y)
	}
}
