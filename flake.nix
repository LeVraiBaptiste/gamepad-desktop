{
  description = "Gamepad to keyboard/mouse mapper with per-application profiles";

  inputs.nixpkgs.url = "github:NixOS/nixpkgs/nixos-unstable";

  outputs = { self, nixpkgs }:
    let
      systems = [ "x86_64-linux" "aarch64-linux" ];
      forAllSystems = f: nixpkgs.lib.genAttrs systems (system: f nixpkgs.legacyPackages.${system});
    in
    {
      packages = forAllSystems (pkgs: rec {
        gamepad-desktop = pkgs.callPackage ./nix/package.nix { };
        default = gamepad-desktop;
      });

      overlays.default = final: prev: {
        gamepad-desktop = final.callPackage ./nix/package.nix { };
      };

      homeManagerModules.default = import ./nix/hm-module.nix self;
      nixosModules.default = ./nix/nixos-module.nix;

      # Building the package runs the Go tests.
      checks = forAllSystems (pkgs: {
        package = self.packages.${pkgs.stdenv.hostPlatform.system}.default;
      });

      devShells = forAllSystems (pkgs: {
        default = pkgs.mkShell {
          packages = with pkgs; [ go gopls gotools evtest ];
          CGO_ENABLED = "0";
        };
      });
    };
}
