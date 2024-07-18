{ config, pkgs, lib, ... }:
let
  helix-with-stuff = pkgs.symlinkJoin {
    name = "helix-with-stuff";
    paths = [ pkgs.helix ];
    buildInputs = [ pkgs.makeWrapper ];
    postBuild = ''
      wrapProgram $out/bin/hx \
        --suffix PATH : ${
          lib.makeBinPath (with pkgs;
            with nodePackages; [
              # LSP Servers
              bash-language-server
              clang-tools
              cmake-language-server
              nixd
              nixfmt-classic
              rust-analyzer
              yaml-language-server
            ])
        }
    '';
  };
in {
  home-manager.users.elxreno.programs.helix = {
    enable = true;
    package = lib.mkIf (config.device == "INFINITY") helix-with-stuff;
    languages = {
      language = [{
        name = "nix";
        formatter.command = "nixfmt";
        language-servers = [ "nixd" ];
        auto-format = true;
      }];
      language-server = { nixd.command = "nixd"; };
    };
  };
}
