// SPDX-License-Identifier: GPL-3.0-or-later

// Package gnomeext embeds the GNOME Shell extension that reports the
// focused window, and installs it for the current user.
package gnomeext

import (
	"embed"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
)

// UUID identifies the extension to GNOME Shell.
const UUID = "gamepad-desktop@levraibaptiste.github.io"

// D-Bus coordinates of the interface exported by extension.js.
const (
	BusName    = "org.gnome.Shell"
	ObjectPath = "/io/github/levraibaptiste/GamepadDesktop"
	Interface  = "io.github.levraibaptiste.GamepadDesktop"
)

//go:embed metadata.json extension.js
var files embed.FS

// Dir returns the per-user install directory of the extension.
func Dir() (string, error) {
	data := os.Getenv("XDG_DATA_HOME")
	if data == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", err
		}
		data = filepath.Join(home, ".local", "share")
	}
	return filepath.Join(data, "gnome-shell", "extensions", UUID), nil
}

// Install copies the extension files and enables the extension in the
// user's GNOME settings. GNOME Shell on Wayland only picks up a newly
// installed extension at the next login.
func Install() (string, error) {
	dir, err := Dir()
	if err != nil {
		return "", err
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}
	for _, name := range []string{"metadata.json", "extension.js"} {
		data, err := files.ReadFile(name)
		if err != nil {
			return "", err
		}
		if err := os.WriteFile(filepath.Join(dir, name), data, 0o644); err != nil {
			return "", err
		}
	}
	_, err = Enable()
	return dir, err
}

// Enable adds the extension to the user's enabled extensions (and removes
// it from the disabled ones) without touching other extensions. It reports
// whether anything changed. The files must already be installed, e.g. by a
// package in XDG_DATA_DIRS. Takes effect at the next login.
func Enable() (bool, error) {
	added, err := updateList("enabled-extensions", true)
	if err != nil {
		return false, err
	}
	removed, err := updateList("disabled-extensions", false)
	return added || removed, err
}

// Uninstall disables the extension and removes its files.
func Uninstall() (string, error) {
	dir, err := Dir()
	if err != nil {
		return "", err
	}
	if _, err := updateList("enabled-extensions", false); err != nil {
		return dir, err
	}
	return dir, os.RemoveAll(dir)
}

// updateList adds UUID to (or removes it from) a string-list key of
// org.gnome.shell, through the gsettings tool shipped with GNOME. It reports
// whether the list changed.
func updateList(key string, present bool) (bool, error) {
	out, err := exec.Command("gsettings", "get", "org.gnome.shell", key).Output()
	if err != nil {
		return false, fmt.Errorf("reading org.gnome.shell %s: %w", key, err)
	}
	list, err := parseStrv(string(out))
	if err != nil {
		return false, fmt.Errorf("reading org.gnome.shell %s: %w", key, err)
	}
	has := slices.Contains(list, UUID)
	switch {
	case present && !has:
		list = append(list, UUID)
	case !present && has:
		list = slices.DeleteFunc(list, func(s string) bool { return s == UUID })
	default:
		return false, nil
	}
	if out, err := exec.Command("gsettings", "set", "org.gnome.shell", key, formatStrv(list)).CombinedOutput(); err != nil {
		return false, fmt.Errorf("writing org.gnome.shell %s: %w: %s", key, err, strings.TrimSpace(string(out)))
	}
	return true, nil
}

// parseStrv parses the GVariant text form of a string array as printed by
// gsettings, e.g. "['a@b', 'c@d']" or "@as []". Extension UUIDs never
// contain quotes, commas or backslashes, so a simple split is enough.
func parseStrv(s string) ([]string, error) {
	s = strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(s), "@as"))
	if !strings.HasPrefix(s, "[") || !strings.HasSuffix(s, "]") {
		return nil, fmt.Errorf("unexpected value %q", s)
	}
	s = strings.TrimSpace(s[1 : len(s)-1])
	if s == "" {
		return nil, nil
	}
	var list []string
	for item := range strings.SplitSeq(s, ",") {
		item = strings.TrimSpace(item)
		if len(item) < 2 || item[0] != '\'' || item[len(item)-1] != '\'' {
			return nil, fmt.Errorf("unexpected item %q", item)
		}
		list = append(list, item[1:len(item)-1])
	}
	return list, nil
}

func formatStrv(list []string) string {
	quoted := make([]string, len(list))
	for i, s := range list {
		quoted[i] = "'" + s + "'"
	}
	return "[" + strings.Join(quoted, ", ") + "]"
}
