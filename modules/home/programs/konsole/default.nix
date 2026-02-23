{
  config,
  namespace,
  lib,
  ...
}:
let
  inherit (lib) mkIf mkEnableOption;
  cfg = config.${namespace}.programs.konsole;
in
{
  options.${namespace}.programs.konsole = {
    enable = mkEnableOption "Whether or not to manage konsole.";
  };

  config = mkIf cfg.enable {
    programs.konsole = {
      enable = true;

      defaultProfile = "default";

      extraConfig = {
        KonsoleWindow.RememberWindowSize = false;

        TabBar = {
          NewTabBehavior = "PutNewTabAfterCurrentTab";
          TabBarVisibility = "AlwaysShowTabBar";
          TabBarPosition = "Bottom";
        };
      };

      profiles.default = {
        font = {
          name = "JetBrains Mono";
          size = 10;
        };

        extraConfig = {
          General = {
            TerminalColumns = 120;
            TerminalRows = 30;
          };
          Scrolling = {
            HistorySize = 50000;
          };
        };
      };
    };
  };
}
