{
  config,
  namespace,
  lib,
  pkgs,
  ...
}:
let
  inherit (lib) mkIf mkEnableOption;
  cfg = config.${namespace}.programs.opencode;
in
{
  options.${namespace}.programs.opencode = {
    enable = mkEnableOption "Whether to manage OpenCode.";
  };

  config = mkIf cfg.enable {
    home.packages = [ pkgs.rtk ];

    programs.opencode = {
      enable = true;
      enableMcpIntegration = true;
      settings.provider.openai.models."gpt-5.5".limit = {
        context = 272000;
        output = 128000;
      };
      settings.plugin = with pkgs.${namespace}; [
        "file://${opencode-dcp.pluginDir}"
        "file://${opencode-working-memory.pluginDir}"

        "file://${pkgs.rtk.src}/hooks/opencode/rtk.ts"
      ];
      tui.plugin = with pkgs.${namespace}; [
        "file://${opencode-working-memory.pluginDir}"
      ];
    };

    xdg.configFile."opencode/dcp.jsonc".text = builtins.toJSON {
      enabled = true;
      autoUpdate = false;
      showUpdateToasts = false;
      compress = {
        maxContextLimit = "70%";
        nudgeForce = "strong";
        protectUserMessages = true;
      };
    };
  };
}
