{
  config,
  namespace,
  lib,
  pkgs,
  inputs,
  ...
}:
let
  inherit (lib) mkIf mkEnableOption;
  cfg = config.${namespace}.programs.noctalia;
in
{
  options.${namespace}.programs.noctalia = {
    enable = mkEnableOption "Whether or not to manage Noctalia shell.";
  };

  config = mkIf cfg.enable {
    programs.noctalia-shell = {
      enable = true;
      package =
        (inputs.noctalia.packages.${pkgs.stdenv.hostPlatform.system}.default.override {
          extraPackages = [
            pkgs.gpu-screen-recorder
            pkgs.sqlite
          ];
        }).overrideAttrs
          (old: {
            buildInputs = (old.buildInputs or [ ]) ++ [ pkgs.qt6.qtwebsockets ];
          });
      settings = {
        general = {
          dimmerOpacity = 0.0;
          clockFormat = "HH:mm:ss";
          enableLockScreenMediaControls = true;
          lockScreenAnimations = true;
          showSessionButtonsOnLockScreen = false;
        };

        bar.widgets = {
          left = [
            {
              id = "Workspace";
              focusedColor = "primary";
              occupiedColor = "none";
              emptyColor = "none";
            }
            {
              id = "SystemMonitor";
              compactMode = false;
              usePadding = true;
              showNetworkStats = true;
            }
            {
              id = "ActiveWindow";
              maxWidth = 400;
            }
            { id = "MediaMini"; }
          ];
          center = [
            { id = "Clock"; }
          ];
          right = [
            { id = "Tray"; }
            {
              id = "KeyboardLayout";
              displayMode = "forceOpen";
            }
            { id = "plugin:usb-drive-manager"; }
            { id = "plugin:tailscale"; }
            { id = "plugin:hassio"; }
            { id = "NotificationHistory"; }
            { id = "plugin:screen-recorder"; }
            {
              id = "Battery";
              showPowerProfiles = true;
            }
            { id = "Volume"; }
            { id = "Brightness"; }
            { id = "ControlCenter"; }
            { id = "KeepAwake"; }
          ];
        };

        sessionMenu.powerOptions = [
          {
            action = "lock";
            enabled = false;
            keybind = "1";
          }
          {
            action = "suspend";
            enabled = true;
            keybind = "2";
          }
          {
            action = "hibernate";
            enabled = false;
            keybind = "3";
          }
          {
            action = "reboot";
            enabled = true;
            keybind = "4";
          }
          {
            action = "logout";
            enabled = true;
            keybind = "5";
          }
          {
            action = "shutdown";
            enabled = true;
            keybind = "6";
          }
          {
            action = "rebootToUefi";
            enabled = false;
            keybind = "7";
          }
        ];

        notifications = {
          enableKeyboardLayoutToast = false;
          lowUrgencyDuration = 3;
          normalUrgencyDuration = 5;
          criticalUrgencyDuration = 7;
        };

        idle = {
          enabled = true;
          screenOffTimeout = 1200;
          screenOffCommand = "niri msg action power-off-monitors";
          resumeScreenOffCommand = "niri msg action power-on-monitors";
          lockTimeout = 600;
          lockCommand = "noctalia-shell ipc call lockScreen lock";
          suspendTimeout = 0;
          customCommands = builtins.toJSON [
            {
              name = "Dim screen";
              timeout = 540;
              command = "brightnessctl set 20%- --save";
              resumeCommand = "brightnessctl --restore";
            }
            {
              name = "Power-saver profile";
              timeout = 1210;
              command = "powerprofilesctl set power-saver";
              resumeCommand = "powerprofilesctl set performance";
            }
          ];
        };

        appLauncher = {
          terminalCommand = "kitty -e";
          enableClipboardHistory = true;
          customLaunchPrefixEnabled = true;
          customLaunchPrefix = "${lib.getExe pkgs.uwsm} app --";
        };
        wallpaper.overviewEnabled = true;
        location.autoLocate = true;
        plugins = {
          autoUpdate = false;
          notifyUpdates = false;
        };
      };

      plugins = {
        version = 2;
        sources = [
          {
            enabled = false;
            name = "Noctalia Plugins";
            url = "https://github.com/noctalia-dev/noctalia-plugins";
          }
        ];

        states = {
          screen-recorder = {
            enabled = true;
            sourceUrl = "https://github.com/noctalia-dev/noctalia-plugins";
          };
          tailscale = {
            enabled = true;
            sourceUrl = "https://github.com/noctalia-dev/noctalia-plugins";
          };
          zed-provider = {
            enabled = true;
            sourceUrl = "https://github.com/noctalia-dev/noctalia-plugins";
          };
          usb-drive-manager = {
            enabled = true;
            sourceUrl = "https://github.com/noctalia-dev/noctalia-plugins";
          };
          hassio = {
            enabled = true;
            sourceUrl = "https://github.com/noctalia-dev/noctalia-plugins";
          };
        };
      };

      pluginSettings = {
        usb-drive-manager = {
          fileBrowser = "thunar";
          terminalCommand = "kitty";
        };

        screen-recorder = {
          directory = "${config.home.homeDirectory}/Videos/Recordings";
          copyToClipboard = true;
          frameRate = "144";
          videoCodec = "hevc";
          colorRange = "full";
          replayEnabled = true;
          replayDuration = "60";
        };
      };
    };

    xdg.configFile = {
      "noctalia/plugins/screen-recorder" = {
        source = inputs.noctalia-plugins + "/screen-recorder";
        recursive = true;
      };
      "noctalia/plugins/tailscale" = {
        source = pkgs.applyPatches {
          name = "tailscale-plugin-patched";
          src = inputs.noctalia-plugins + "/tailscale";
          patches = [ ./patches/tailscale-icon-color.patch ];
          patchFlags = [ "-p2" ];
        };
        recursive = true;
      };
      "noctalia/plugins/zed-provider" = {
        source = inputs.noctalia-plugins + "/zed-provider";
        recursive = true;
      };
      "noctalia/plugins/usb-drive-manager" = {
        source = inputs.noctalia-plugins + "/usb-drive-manager";
        recursive = true;
      };
      "noctalia/plugins/hassio" = {
        source = pkgs.applyPatches {
          name = "hassio-plugin-patched";
          src = inputs.noctalia-plugins + "/hassio";
          patches = [ ./patches/hassio-icon-color.patch ];
          patchFlags = [ "-p2" ];
        };
        recursive = true;
      };
    };
  };
}
