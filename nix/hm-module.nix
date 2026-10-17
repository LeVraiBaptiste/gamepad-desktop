# home-manager module. Imported from the flake as `import ./hm-module.nix self`.
self:
{ config, lib, pkgs, ... }:

let
  cfg = config.services.gamepad-desktop;
  toml = pkgs.formats.toml { };

  configSource =
    if cfg.configFile != null then cfg.configFile
    else toml.generate "gamepad-desktop.toml" cfg.settings;
in
{
  options.services.gamepad-desktop = {
    enable = lib.mkEnableOption "gamepad-desktop, a gamepad to keyboard and mouse mapper";

    package = lib.mkOption {
      type = lib.types.package;
      default = self.packages.${pkgs.stdenv.hostPlatform.system}.default;
      defaultText = lib.literalExpression "gamepad-desktop.packages.\${system}.default";
      description = "The gamepad-desktop package to use.";
    };

    settings = lib.mkOption {
      inherit (toml) type;
      default = { };
      example = lib.literalExpression ''
        {
          settings.sensitivity = 1400;
          default = {
            BTN_SOUTH = "BTN_LEFT";
            BTN_EAST = "BTN_RIGHT";
            RSTICK = "cursor";
            LSTICK = "scroll";
            BTN_TR = "alt-tab";
            "BTN_SELECT+BTN_START" = "pause";
          };
          app.zen.BTN_NORTH = "ctrl+t";
        }
      '';
      description = ''
        Configuration written to {file}`$XDG_CONFIG_HOME/gamepad-desktop/config.toml`.
        See `examples/config.toml` in the repository for the format. When
        empty, the built-in configuration is used.
      '';
    };

    configFile = lib.mkOption {
      type = lib.types.nullOr lib.types.path;
      default = null;
      example = lib.literalExpression "./gamepad.toml";
      description = "A configuration file to use instead of {option}`settings`.";
    };

    extraArgs = lib.mkOption {
      type = lib.types.listOf lib.types.str;
      default = [ ];
      example = [ "--verbose" ];
      description = "Extra command-line arguments for the daemon.";
    };

    gnomeExtension.enable = lib.mkOption {
      type = lib.types.bool;
      default = true;
      description = ''
        Whether to enable the bundled GNOME Shell extension that reports the
        focused application, needed for per-application profiles on GNOME.
        The daemon adds it to the user's enabled extensions when it starts,
        keeping the others, so extensions enabled by hand are not lost.
        Takes effect at the next login.
      '';
    };
  };

  config = lib.mkIf cfg.enable {
    assertions = [{
      assertion = cfg.configFile == null || cfg.settings == { };
      message = "services.gamepad-desktop: set either `settings` or `configFile`, not both.";
    }];

    # Also puts the extension in share/gnome-shell/extensions, where GNOME
    # Shell looks for it through XDG_DATA_DIRS.
    home.packages = [ cfg.package ];

    # Written to the usual location too, so a manual
    # `gamepad-desktop --verbose` run uses the same configuration.
    xdg.configFile."gamepad-desktop/config.toml" =
      lib.mkIf (cfg.configFile != null || cfg.settings != { }) { source = configSource; };

    systemd.user.services.gamepad-desktop = {
      Unit = {
        Description = "Gamepad to keyboard and mouse mapper";
        Documentation = [ "https://github.com/LeVraiBaptiste/gamepad-desktop" ];
        PartOf = [ "graphical-session.target" ];
        After = [ "graphical-session.target" ];
        X-Restart-Triggers = [ "${configSource}" ];
      };
      Service = {
        ExecStart = lib.escapeShellArgs (
          [ (lib.getExe cfg.package) ]
          ++ lib.optionals (cfg.configFile != null || cfg.settings != { }) [ "--config" "${configSource}" ]
          ++ lib.optional cfg.gnomeExtension.enable "--enable-extension"
          ++ cfg.extraArgs
        );
        # Only gsettings is run, to enable the GNOME extension. Without the
        # dconf GIO module it would silently use an in-memory backend.
        Environment = [
          "PATH=${lib.makeBinPath [ pkgs.glib ]}"
          "GIO_EXTRA_MODULES=${pkgs.dconf.lib}/lib/gio/modules"
        ];
        Restart = "on-failure";
        RestartSec = 3;
        MemoryMax = "64M";
        NoNewPrivileges = true;
      };
      Install.WantedBy = [ "graphical-session.target" ];
    };
  };
}
