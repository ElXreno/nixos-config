{
  config,
  namespace,
  lib,
  pkgs,
  ...
}:
let
  inherit (lib) mkIf mkEnableOption;
  cfg = config.${namespace}.programs.helix;
  lsp = import ../_lsp.nix { inherit pkgs lib; };

  helix-with-stuff = pkgs.symlinkJoin {
    name = "helix-with-stuff";
    paths = [ pkgs.helix ];
    buildInputs = [ pkgs.makeWrapper ];
    postBuild = ''
      wrapProgram $out/bin/hx --suffix PATH : ${lib.makeBinPath lsp.packages}
    '';
  };
in
{
  options.${namespace}.programs.helix = {
    enable = mkEnableOption "managing helix";
    setAsDefault = mkEnableOption "helix as default editor" // {
      default = true;
    };
  };

  config = mkIf cfg.enable {
    programs.helix = {
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

    home.sessionVariables.EDITOR = mkIf cfg.setAsDefault "hx";
  };
}
