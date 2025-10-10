{
  config,
  namespace,
  lib,
  ...
}:
let
  inherit (lib)
    mkIf
    mkEnableOption
    mkOption
    types
    ;
  cfg = config.${namespace}.desktop-environments.plasma;
in
{
  options.${namespace}.desktop-environments.plasma = {
    enable = mkEnableOption "Whether or not to manage plasma.";
    wayland = mkOption {
      type = types.bool;
      default = true;
      description = "Whether to enable wayland.";
    };
  };

  config = mkIf cfg.enable {
    home.sessionVariables = mkIf cfg.wayland {
      NIXOS_OZONE_WL = 1;
    };

    programs.plasma = {
      enable = true;
      configFile = {
        baloofilerc = {
          "Basic Settings"."Indexing-Enabled" = false;
        };

        dolphinrc = {
          General = {
            GlobalViewProps = false;
            ShowFullPath = true;
          };
        };

        kdeglobals = {
          Icons.Theme = "Papirus";
          KDE.SingleClick = true;
        };

        kscreenlockerrc = {
          Daemon = {
            Timeout = 10; # min
            LockGrace = 7; # sec
          };
        };

        kcminputrc."Libinput.10182.480.GXTP7863:00 27C6:01E0 Touchpad" = {
          NaturalScroll = true;
          ScrollFactor = 0.5;
          TapToClick = true;
        };

        ksmserverrc.General.loginMode = "restoreSavedSession";

        konsolerc = {
          "Desktop Entry".DefaultProfile = "default.profile";

          KonsoleWindow.RememberWindowSize = false;

          TabBar = {
            NewTabBehavior = "PutNewTabAfterCurrentTab";
            TabBarVisibility = "AlwaysShowTabBar";
            TabBarPosition = "Bottom";
          };
        };
        "../.local/share/konsole/default.profile" = {
          Appearance.Font = "JetBrains Mono,10,-1,5,50,0,0,0,0,0";
          General = {
            Name = "default";
            TerminalColumns = 120;
            TerminalRows = 30;
          };
          Scrolling = {
            HistorySize = 50000;
          };
        };

        kwinrc = {
          NightColor.Active = true;
          Xwayland.Scale = 1;
        };
      };
    };
  };
}
