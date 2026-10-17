// SPDX-License-Identifier: GPL-3.0-or-later

import Gio from 'gi://Gio';
import {Extension} from 'resource:///org/gnome/shell/extensions/extension.js';

// Read-only on purpose: only the class of the focused window is exposed,
// not its title or any way to act on windows.
const IFACE = `
<node>
  <interface name="io.github.levraibaptiste.GamepadDesktop">
    <method name="FocusedWindow">
      <arg type="s" direction="out" name="wm_class"/>
      <arg type="s" direction="out" name="wm_class_instance"/>
    </method>
  </interface>
</node>`;

const OBJECT_PATH = '/io/github/levraibaptiste/GamepadDesktop';

export default class GamepadDesktopExtension extends Extension {
    enable() {
        this._dbus = Gio.DBusExportedObject.wrapJSObject(IFACE, this);
        this._dbus.export(Gio.DBus.session, OBJECT_PATH);
    }

    disable() {
        this._dbus?.unexport();
        this._dbus = null;
    }

    // Returns empty strings when no window has the focus.
    FocusedWindow() {
        const win = global.display.get_focus_window();
        return [win?.get_wm_class() ?? '', win?.get_wm_class_instance() ?? ''];
    }
}
