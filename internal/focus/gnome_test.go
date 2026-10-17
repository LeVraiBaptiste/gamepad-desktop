// SPDX-License-Identifier: GPL-3.0-or-later

package focus

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/godbus/dbus/v5"

	"github.com/LeVraiBaptiste/gamepad-desktop/internal/gnomeext"
)

// fakeShell mimics extension.js.
type fakeShell struct {
	class, instance string
	delay           time.Duration
}

func (f fakeShell) FocusedWindow() (string, string, *dbus.Error) {
	time.Sleep(f.delay)
	return f.class, f.instance, nil
}

// serve exports f on a private session bus connection and returns a
// provider talking to it. Skips when there is no session bus (e.g. CI).
func serve(t *testing.T, f *fakeShell) *GNOME {
	t.Helper()
	server, err := dbus.ConnectSessionBus()
	if err != nil {
		t.Skipf("no session bus: %v", err)
	}
	client, err := dbus.ConnectSessionBus()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { client.Close(); server.Close() })
	if f != nil {
		if err := server.Export(*f, gnomeext.ObjectPath, gnomeext.Interface); err != nil {
			t.Fatal(err)
		}
	}
	return newGNOME(client, server.Names()[0])
}

func TestFocused(t *testing.T) {
	g := serve(t, &fakeShell{class: "zen", instance: "zen-beta"})
	w, ok, err := g.Focused(context.Background())
	if err != nil || !ok || w != (Window{WMClass: "zen", Instance: "zen-beta"}) {
		t.Fatalf("got %+v ok=%v err=%v", w, ok, err)
	}
}

func TestNoFocusedWindow(t *testing.T) {
	g := serve(t, &fakeShell{})
	if _, ok, err := g.Focused(context.Background()); ok || err != nil {
		t.Fatalf("got ok=%v err=%v, want no window and no error", ok, err)
	}
}

func TestExtensionMissing(t *testing.T) {
	g := serve(t, nil)
	if _, _, err := g.Focused(context.Background()); !errors.Is(err, ErrUnavailable) {
		t.Fatalf("got %v, want ErrUnavailable", err)
	}
}

func TestTimeout(t *testing.T) {
	g := serve(t, &fakeShell{class: "zen", delay: time.Second})
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	start := time.Now()
	_, _, err := g.Focused(ctx)
	if err == nil || errors.Is(err, ErrUnavailable) {
		t.Fatalf("got %v, want a timeout error", err)
	}
	if d := time.Since(start); d > 500*time.Millisecond {
		t.Errorf("call took %v despite the 50ms timeout", d)
	}
}
