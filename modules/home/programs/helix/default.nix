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
  options.${namespace}.programs.helix = {
    enable = mkEnableOption "Whether or not to manage helix.";
    setAsDefault = mkEnableOption "Whether to enable helix as default editor." // {
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
