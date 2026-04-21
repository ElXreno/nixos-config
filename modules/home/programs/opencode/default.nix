{
  config,
  namespace,
  lib,
  pkgs,
  ...
}:
let
  inherit (lib)
    mkIf
    mkEnableOption
    mkOption
    types
    ;
  cfg = config.${namespace}.programs.opencode;
in
{
  options.${namespace}.programs.opencode = {
    enable = mkEnableOption "Whether or not to manage opencode.";
    plugins = mkOption {
      type = with types; listOf str;
      default = [
        "${pkgs.opencode-claude-auth}/lib/node_modules/opencode-claude-auth"
        "${pkgs.oh-my-opencode}/lib/oh-my-opencode"
      ];
    };
  };

  config = mkIf cfg.enable {
    programs.opencode = {
      enable = true;
      web = {
        enable = true;
        extraArgs = [
          "--hostname"
          "0.0.0.0"
          "--port"
          "4096"
        ];
      };

      settings = {
        plugin = cfg.plugins;
      };
    };
  };
}
