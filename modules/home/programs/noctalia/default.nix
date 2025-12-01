{
  config,
  namespace,
  lib,
  pkgs,
  ...
}:
let
  inherit (lib) mkIf mkEnableOption;
  cfg = config.${namespace}.programs.noctalia;
in
{
  options.${namespace}.programs.noctalia = {
    enable = mkEnableOption "Whether or not to manage noctalia.";
  };

  config = mkIf cfg.enable {
    programs = {
      noctalia-shell = {
        enable = true;
        package = pkgs.noctalia-shell;
        systemd.enable = true;
        settings = {
          bar = {
            density = "default";
            position = "top";
            showCapsule = false;
            outerCorners = false;
            widgets = {
              left = [
                {
                  id = "ControlCenter";
                  useDistroLogo = true;
                }
                {
                  id = "SystemMonitor";
                  showMemoryAsPercent = true;
                  showNetworkStats = true;
                }
                { id = "ActiveWindow"; }
                { id = "MediaMini"; }
              ];
              center = [
                { id = "Workspace"; }
              ];
              right = [
                { id = "Tray"; }
                {
                  id = "KeyboardLayout";
                  displayMode = "forceOpen";
                }
                { id = "ScreenRecorder"; }
                { id = "NotificationHistory"; }
                { id = "Battery"; }
                { id = "Bluetooth"; }
                { id = "WiFi"; }
                { id = "Microphone"; }
                { id = "Volume"; }
                { id = "Brightness"; }
                { id = "Clock"; }
              ];
            };
          };

          appLauncher = {
            enableClipboardHistory = true;
            terminalCommand = lib.getExe pkgs.kitty;
          };

          dock.enabled = false;

          location = {
            name = "Vitebsk";
          };

          notifications = {
            location = "bottom_right";
            enableKeyboardLayoutToast = false;
          };

          wallpaper = {
            directory = "${pkgs.${namespace}.custom-wallpapers}";
          };
        };
      };
    };
  };
}
