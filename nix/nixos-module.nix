# NixOS module: the system-side permissions gamepad-desktop needs. The
# daemon itself runs per user, from the home-manager module.
{ config, lib, pkgs, ... }:

let
  cfg = config.hardware.gamepad-desktop;

  # A rules package rather than extraRules (99-): "uaccess" must be set
  # before 73-seat-late.rules.
  udevRules = pkgs.writeTextDir "lib/udev/rules.d/70-gamepad-desktop.rules"
    (builtins.readFile ../contrib/70-gamepad-desktop.rules);
in
{
  options.hardware.gamepad-desktop.enable = lib.mkEnableOption ''
    the permissions gamepad-desktop needs: the uinput module, and access to
    /dev/uinput and to gamepads for the user of the active session'';

  config = lib.mkIf cfg.enable {
    hardware.uinput.enable = true;
    services.udev.packages = [ udevRules ];
  };
}
