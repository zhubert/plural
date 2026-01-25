{
  description = "Plural - TUI for managing multiple concurrent Claude Code sessions";

  inputs = {
    nixpkgs.url = "github:NixOS/nixpkgs/nixos-unstable";
    flake-utils.url = "github:numtide/flake-utils";
  };

  outputs = { self, nixpkgs, flake-utils }:
    flake-utils.lib.eachDefaultSystem (system:
      let
        pkgs = nixpkgs.legacyPackages.${system};
        version = "0.2.3";
      in
      {
        packages = {
          plural = pkgs.buildGoModule {
            pname = "plural";
            inherit version;
            src = ./.;

            vendorHash = "sha256-6OQZVy3MHBITk0GWVx2t0XKz39BV3NscGmMVwhcK4oQ=";

            # Tests require filesystem access (home directory) which isn't available in Nix sandbox
            doCheck = false;

            ldflags = [
              "-s" "-w"
            ];

            meta = with pkgs.lib; {
              description = "TUI for managing multiple concurrent Claude Code sessions";
              homepage = "https://github.com/zhubert/plural";
              license = licenses.mit;
              mainProgram = "plural";
            };
          };

          default = self.packages.${system}.plural;
        };

        apps = {
          plural = {
            type = "app";
            program = "${self.packages.${system}.plural}/bin/plural";
          };
          default = self.apps.${system}.plural;
        };
      }
    );
}
