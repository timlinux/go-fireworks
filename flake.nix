{
  description = "A terminal fireworks show using Go, tcell, and particle effects";

  inputs = {
    nixpkgs.url = "github:NixOS/nixpkgs/nixos-unstable";
    flake-utils.url = "github:numtide/flake-utils";
  };

  outputs = { self, nixpkgs, flake-utils }:
    flake-utils.lib.eachDefaultSystem (system:
      let
        pkgs = nixpkgs.legacyPackages.${system};
      in
      {
        packages = {
          default = pkgs.buildGoModule {
            pname = "tim-particles";
            version = "0.1.0";

            src = ./.;

            vendorHash = "sha256-RCUC5rLpBhqFeGsGOaT3OVNZZFeBlql8ujgx4RtfSE8=";

            meta = with pkgs.lib; {
              description = "Terminal fireworks show";
              homepage = "https://github.com/timlinux/tim-particles";
              license = licenses.mit;
              maintainers = [ ];
            };
          };
        };

        apps.default = {
          type = "app";
          program = "${self.packages.${system}.default}/bin/tim-particles";
        };

        devShells.default = pkgs.mkShell {
          buildInputs = with pkgs; [
            go
            gopls
            gotools
            go-tools
          ];

          shellHook = ''
            echo "Go development environment loaded"
            echo "Run 'go run main.go' to start the fireworks show"
            echo "Or use 'nix build' and 'nix run' to build and run with Nix"
          '';
        };
      }
    );
}
