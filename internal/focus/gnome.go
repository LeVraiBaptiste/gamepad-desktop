// SPDX-License-Identifier: GPL-3.0-or-later

package focus

import (
	"context"
	"errors"
	"fmt"

	"github.com/godbus/dbus/v5"

	"github.com/LeVraiBaptiste/gamepad-desktop/internal/gnomeext"
)

// GNOME queries the gamepad-desktop GNOME Shell extension over D-Bus.
type GNOME struct {
	conn *dbus.Conn
	obj  dbus.BusObject
}

// NewGNOME connects to the session bus.
func NewGNOME() (*GNOME, error) {
	conn, err := dbus.ConnectSessionBus()
	if err != nil {
		return nil, fmt.Errorf("cannot connect to the session bus: %w (%w)", err, ErrUnavailable)
	}
	return newGNOME(conn, gnomeext.BusName), nil
}

func newGNOME(conn *dbus.Conn, dest string) *GNOME {
	return &GNOME{conn: conn, obj: conn.Object(dest, gnomeext.ObjectPath)}
}

// Focused implements Provider.
func (g *GNOME) Focused(ctx context.Context) (Window, bool, error) {
	var w Window
	err := g.obj.CallWithContext(ctx, gnomeext.Interface+".FocusedWindow", 0).Store(&w.WMClass, &w.Instance)
	if err != nil {
		if dbusErr, ok := errors.AsType[dbus.Error](err); ok {
			switch dbusErr.Name {
			case "org.freedesktop.DBus.Error.UnknownMethod",
				"org.freedesktop.DBus.Error.UnknownObject",
				"org.freedesktop.DBus.Error.UnknownInterface",
				"org.freedesktop.DBus.Error.ServiceUnknown":
				return Window{}, false, fmt.Errorf("the gamepad-desktop GNOME extension is not running; "+
					"run `gamepad-desktop install-extension`, then log out and back in (%w)", ErrUnavailable)
			}
		}
		return Window{}, false, fmt.Errorf("querying the GNOME extension: %w", err)
	}
	if w.WMClass == "" && w.Instance == "" {
		return Window{}, false, nil
	}
	return w, true, nil
}

// Close implements Provider.
func (g *GNOME) Close() error { return g.conn.Close() }
