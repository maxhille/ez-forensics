{
  description = "EZ Forensics";

  inputs = {
    nixpkgs.url = "github:NixOS/nixpkgs/nixos-unstable";
    flake-utils.url = "github:numtide/flake-utils";
    proc = {
      url = "git+https://codeberg.org/maxhille/proc";
      inputs.nixpkgs.follows = "nixpkgs";
    };
    remake = {
      url = "git+https://codeberg.org/maxhille/remake";
      inputs.nixpkgs.follows = "nixpkgs";
    };
    elm-esm-src = {
      url = "github:ChristophP/elm-esm";
      flake = false;
    };
  };

  outputs =
    {
      self,
      nixpkgs,
      flake-utils,
      elm-esm-src,
      proc,
      remake,
    }:
    flake-utils.lib.eachDefaultSystem (
      system:
      let
        pkgs = import nixpkgs { inherit system; };
        elm-esm = pkgs.buildNpmPackage {
          pname = "elm-esm";
          version = "1.1.4";

          src = elm-esm-src;

          npmDepsHash = "sha256-ZAwXpKvnSYdEOmcvsXl6j7G1TsdOz+9Q2SKE3seuoNg=";
          npmFlags = [ "--ignore-scripts" ];

          # Patch the Elm binary lookup code
          postPatch = ''
            # Safe-guard the spawnSync call inside the nested dependency
            substituteInPlace src/run.js \
              --replace-fail 'await findElmBinary()' '"${pkgs.elmPackages.elm}/bin/elm"'
          '';

          dontNpmBuild = true;
        };
      in
      {
        devShells.default = pkgs.mkShell {
          buildInputs = [
            elm-esm
            proc.packages.${system}.default
            remake.packages.${system}.default
            pkgs.gnumake
            pkgs.elmPackages.elm
            pkgs.elmPackages.elm-test
            pkgs.go
            pkgs.sqlc
            pkgs.air
          ];
        };
      }
    );
}
