{ pkgs, lib, ... }:
let
  helix-with-stuff = pkgs.symlinkJoin {
    name = "helix-with-stuff";
    paths = [ pkgs.helix ];
    buildInputs = [ pkgs.makeWrapper ];
    postBuild = ''
      wrapProgram $out/bin/hx \
        --suffix PATH : ${
          lib.makeBinPath (
            with pkgs;
            with nodePackages;
            [
              # LSP Servers
              bash-language-server
              nixd
              yaml-language-server
            ]
          )
        }
    '';
  };
in
{
  home-manager.users.elxreno.programs.helix = {
    enable = true;
    package = helix-with-stuff;
    settings = {
      keys =
        let
          defaultKeys = {
            "C-s" = ":w";
          };
        in
        {
          normal = defaultKeys;
          insert = defaultKeys;
        };
    };
    languages = {
      language = [
        {
          name = "nix";
          formatter.command = "nixfmt";
          language-servers = [ "nixd" ];
          auto-format = true;
        }
        {
          name = "rust";
          auto-format = true;
        }
      ];
      language-server = {
        nixd.command = "nixd";
        rust-analyzer = {
          config = {
            diagnostics.experimental.enable = true;
            check.features = "all";
          };
        };
      };
    };
  };
}
