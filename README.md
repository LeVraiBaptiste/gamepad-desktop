# gamepad-desktop

Use a gamepad as a keyboard and mouse on Linux, with mappings that follow
the focused application. Built for GNOME on Wayland, where no other tool can
tell which application has the focus.

- **Per-application profiles**, keyed by `wm_class`, inheriting a default profile.
- **Sticks** move the pointer or scroll (high-resolution wheel), with a radial
  dead zone and a response curve.
- **Buttons, d-pad and triggers** send keys, chords (`ctrl+t`, held while the
  button is), sequences (`ctrl+b, k`) or mouse buttons.
- **Combos** (`"BTN_SELECT+BTN_START" = "pause"`), a stateful **alt-tab**, and a
  **pause** that hands the gamepad back to games.
- **Hot-plug**: waits for the gamepad and picks it up again when reconnected.
- **Light**: a single static binary, about 7 MB of RAM, and no wakeups at all
  while the gamepad is idle.

## How it works

The daemon grabs the gamepad (so the desktop does not see it twice) and
emits events through two virtual devices, a keyboard and a mouse, created
with uinput. This works on any compositor.

GNOME Shell on Wayland exposes no way for a program to know the focused
window. gamepad-desktop ships a tiny GNOME Shell extension, embedded in the
binary, that exposes exactly one read-only D-Bus method returning the
`wm_class` of the focused window: no title, nothing else. Without it (or on
another desktop), the default profile is used for everything.

The focus is only queried when a button is pressed or a stick starts moving.

## Installation

### Nix (flake)

```nix
# flake.nix
inputs.gamepad-desktop.url = "github:LeVraiBaptiste/gamepad-desktop";
```

NixOS, for the permissions (uinput, and access to `/dev/uinput` and the
gamepad for the logged-in user):

```nix
imports = [ inputs.gamepad-desktop.nixosModules.default ];
hardware.gamepad-desktop.enable = true;
```

home-manager, for the daemon (a systemd user service) and the extension:

```nix
imports = [ inputs.gamepad-desktop.homeManagerModules.default ];
services.gamepad-desktop = {
  enable = true;
  settings = {
    default = {
      BTN_SOUTH = "BTN_LEFT";
      RSTICK = "cursor";
      LSTICK = "scroll";
    };
    app.zen.BTN_NORTH = "ctrl+t";
  };
};
```

The service adds the extension to your enabled GNOME extensions without
touching the others. Log out and back in once so GNOME Shell loads it.

The package alone is `inputs.gamepad-desktop.packages.${system}.default`
(also available through `overlays.default`).

### Debian, Ubuntu, Fedora, Arch

Download the package for your distribution from the [releases](../../releases)
and install it:

```sh
sudo apt install ./gamepad-desktop_*_amd64.deb         # Debian, Ubuntu
sudo dnf install ./gamepad-desktop-*.x86_64.rpm        # Fedora, openSUSE
sudo pacman -U ./gamepad-desktop-*-x86_64.pkg.tar.zst  # Arch
```

The package installs the binary, the udev rule (see below), loads uinput, and
ships the GNOME extension and a systemd user service. Then, as your user:

```sh
systemctl --user enable --now gamepad-desktop
```

The service enables the extension at start; log out and back in once so GNOME
Shell loads it.

### Other distributions

1. Get the binary: download it from the
   [releases](../../releases), or `go install github.com/LeVraiBaptiste/gamepad-desktop/cmd/gamepad-desktop@latest`
   (Go 1.26 or later, no C toolchain needed). Put it in `/usr/local/bin`.
2. Permissions: load the uinput module and install the udev rule.
   ```sh
   echo uinput | sudo tee /etc/modules-load.d/uinput.conf
   sudo modprobe uinput
   sudo cp contrib/70-gamepad-desktop.rules /etc/udev/rules.d/
   sudo udevadm control --reload && sudo udevadm trigger
   ```
3. On GNOME, install the extension, then log out and back in:
   ```sh
   gamepad-desktop install-extension
   ```
4. Run it as a user service:
   ```sh
   cp contrib/gamepad-desktop.service ~/.config/systemd/user/
   systemctl --user enable --now gamepad-desktop
   ```

The udev rule gives the user of the active local session access to
`/dev/uinput`, which allows injecting keyboard and mouse input. This is what
the daemon needs; make sure it is acceptable on your machine.

## Configuration

The configuration is read from `$XDG_CONFIG_HOME/gamepad-desktop/config.toml`,
then `/etc/gamepad-desktop/config.toml`. Without one, a built-in configuration
is used:

| Input | Action |
|---|---|
| A (`BTN_SOUTH`) | left click |
| B (`BTN_EAST`) | right click |
| Right stick click (`BTN_THUMBR`) | middle click |
| Right stick | pointer |
| Left stick | scroll |
| RB (`BTN_TR`) | alt-tab: hold, move with the d-pad, release to switch |
| Start | Enter |
| Back, then Start | pause / resume |
| D-pad | arrow keys |

[`examples/config.toml`](examples/config.toml) documents every setting and the
full syntax. A short version:

```toml
[settings]
sensitivity = 1200   # pointer speed, px/s at full deflection

[default]
BTN_SOUTH = "BTN_LEFT"
RSTICK    = "cursor"
"BTN_SELECT+BTN_START" = "pause"

# Inherits [default]; only overrides what it redefines.
[app.kitty]
DPAD_UP = "ctrl+b, k"   # tmux prefix, then k
```

To find the names to use:

- `gamepad-desktop --verbose` logs every gamepad input, the `wm_class` of the
  focused window and the action taken;
- `gamepad-desktop --check` validates the configuration and exits.

## Usage

```
gamepad-desktop [flags]              run the daemon
gamepad-desktop install-extension    install and enable the GNOME extension
gamepad-desktop uninstall-extension  remove it

  --config FILE       use this configuration file
  --device PATH       use this event device instead of auto-detecting
  --check             validate the configuration and exit
  --verbose           log every event and the action it triggers
  --enable-extension  enable the GNOME extension at start (for packaged installs)
  --version           print the version
```

## Development

```sh
nix develop      # Go, gopls, evtest
go test ./...
nix flake check  # builds the package and runs the tests
```

Tested with an Xbox 360 controller (xpad) on GNOME 50. The extension declares
support for GNOME 45 to 50.

## License

GPL-3.0-or-later. See [LICENSE](LICENSE).
