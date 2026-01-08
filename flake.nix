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

            nativeBuildInputs = with pkgs; [ pkg-config makeWrapper ];
            buildInputs = with pkgs; [ pulseaudio ];

            # Ensure pacat is available at runtime via PATH
            postInstall = ''
              wrapProgram $out/bin/tim-particles \
                --prefix PATH : ${pkgs.lib.makeBinPath [ pkgs.pulseaudio ]}
            '';

            meta = with pkgs.lib; {
              description = "Audio-reactive terminal fireworks show";
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
            pulseaudio
          ];

          shellHook = ''
            echo "Go development environment loaded"
            echo "Audio-reactive fireworks - requires PulseAudio/PipeWire"
            echo "Run 'go run *.go' to start the fireworks show"
            echo "Or use 'nix build' and 'nix run' to build and run with Nix"
          '';
        };
      }
    );
}
