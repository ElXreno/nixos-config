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

    ${namespace} = {
      programs.konsole.enable = true;
    };

    programs.plasma = {
      enable = true;

      session.sessionRestore.restoreOpenApplicationsOnLogin = "onLastLogout";

      workspace = {
        clickItemTo = "open";
        lookAndFeel = "org.kde.breeze.desktop";
        cursor.theme = "breeze_cursors";
        iconTheme = "Papirus";
        enableMiddleClickPaste = false;
      };

      hotkeys.commands."launch-konsole" = {
        name = "Launch Konsole";
        key = "Ctrl+Alt+T";
        command = "konsole";
      };

      powerdevil = {
        general.pausePlayersOnSuspend = true;

        AC = {
          autoSuspend.action = "nothing";
          inhibitLidActionWhenExternalMonitorConnected = true;

          dimDisplay = {
            enable = true;
            idleTimeout = 600;
          };

          turnOffDisplay.idleTimeout = 900;

          powerButtonAction = "sleep";
          powerProfile = "performance";
          whenLaptopLidClosed = "turnOffScreen";
          whenSleepingEnter = "standby";
        };

        battery = {
          autoSuspend = {
            action = "sleep";
            idleTimeout = 900;
          };
          inhibitLidActionWhenExternalMonitorConnected = true;

          dimDisplay = {
            enable = true;
            idleTimeout = 300;
          };

          turnOffDisplay.idleTimeout = 600;

          powerButtonAction = "sleep";
          powerProfile = "balanced";
          whenLaptopLidClosed = "turnOffScreen";
          whenSleepingEnter = "standby";
        };
      };

      kscreenlocker = {
        autoLock = true;
        lockOnResume = true;
        passwordRequired = true;
        timeout = 30; # minutes
        passwordRequiredDelay = 7; # seconds
      };

      kwin.nightLight = {
        enable = config.home.username != "alena";
        mode = "automatic";
      };

      input.touchpads = [
        {
          enable = true;
          name = "GXTP7863:00 27C6:01E0 Touchpad";
          vendorId = "27c6";
          productId = "01e0";

          accelerationProfile = "default";
          rightClickMethod = "twoFingers";
          scrollMethod = "twoFingers";

          pointerSpeed = 0;
          scrollSpeed = 0.5;

          disableWhileTyping = true;
          leftHanded = false;
          middleButtonEmulation = true;
          naturalScroll = true;
          tapToClick = true;
        }
      ];

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
      };
    };
  };
}
