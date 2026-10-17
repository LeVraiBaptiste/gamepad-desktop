{ lib, buildGoModule }:

let
  uuid = "gamepad-desktop@levraibaptiste.github.io";
in
buildGoModule (finalAttrs: {
  pname = "gamepad-desktop";
  version = "0.1.0-dev";

  src = lib.fileset.toSource {
    root = ../.;
    fileset = lib.fileset.unions [
      ../go.mod
      ../go.sum
      ../cmd
      ../internal
      ../examples # read by the config tests
    ];
  };

  vendorHash = "sha256-lvDD6CKMU2hFLiSVX15G7XCRuiUtu/csIL6nzZUbq9k=";

  subPackages = [ "cmd/gamepad-desktop" ];
  env.CGO_ENABLED = 0;
  ldflags = [ "-s" "-w" "-X main.version=${finalAttrs.version}" ];

  # Test every package, not only subPackages. D-Bus tests skip themselves
  # without a session bus, as in the build sandbox.
  checkPhase = ''
    runHook preCheck
    go test ./...
    runHook postCheck
  '';

  # GNOME Shell finds extensions in $XDG_DATA_DIRS/gnome-shell/extensions,
  # so installing this package in a profile is enough to make it available.
  postInstall = ''
    install -Dm644 -t $out/share/gnome-shell/extensions/${uuid} \
      internal/gnomeext/metadata.json internal/gnomeext/extension.js
  '';

  passthru.extensionUuid = uuid;

  meta = {
    description = "Use a gamepad as keyboard and mouse, with per-application mappings, on GNOME Wayland";
    homepage = "https://github.com/LeVraiBaptiste/gamepad-desktop";
    license = lib.licenses.gpl3Plus;
    mainProgram = "gamepad-desktop";
    platforms = lib.platforms.linux;
  };
})
