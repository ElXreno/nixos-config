{
  config,
  osConfig,
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
    getExe
    getExe'
    ;
  cfg = config.${namespace}.roles.tablet;

  niriAction = action: "${getExe osConfig.programs.niri.package} msg action ${action}";

  wvkbdHardwareKeyboard = pkgs.writeShellApplication {
    name = "wvkbd-hardware-keyboard";
    runtimeInputs = with pkgs; [
      coreutils
      gnugrep
      systemd
    ];
    text = ''
      has_external_keyboard() {
        for dev in /sys/class/input/event*; do
          bus=$(cat "$dev/device/id/bustype" 2>/dev/null) || continue
          case $bus in
            0003 | 0005) ;;
            *) continue ;;
          esac
          udevadm info --query=property "$dev" | grep -qx 'ID_INPUT_KEYBOARD=1' && return 0
        done
        return 1
      }

      sync_keyboard() {
        if has_external_keyboard; then
          systemctl --user stop wvkbd.service
        else
          systemctl --user start wvkbd.service
        fi
      }

      sync_keyboard
      udevadm monitor --udev --subsystem-match=input | while read -r _; do
        sync_keyboard
      done
    '';
  };

  gestures = [
    "3,RL,*,*,R,${niriAction "focus-column-right"}"
    "3,LR,*,*,R,${niriAction "focus-column-left"}"
    "3,DU,*,*,R,${niriAction "focus-workspace-down"}"
    "3,UD,*,*,R,${niriAction "focus-workspace-up"}"
    "4,DU,*,*,R,${niriAction "toggle-overview"}"
    "1,DU,B,*,R,${getExe' pkgs.systemd "systemctl"} --user kill --signal=SIGRTMIN wvkbd.service"
  ];
in
{
  options.${namespace}.roles.tablet = {
    enable = mkEnableOption "tablet configuration" // {
      default = osConfig.${namespace}.roles.tablet.enable;
    };

    touchscreen = mkOption {
      type = types.nullOr types.str;
      default = null;
      example = "/dev/input/by-id/usb-Atmel_Atmel_maXTouch_Digitizer-event-if00";
      description = "Touchscreen event device that lisgd reads gestures from.";
    };
  };

  config = mkIf cfg.enable {
    systemd.user.services = {
      wvkbd = {
        Unit = {
          Description = "On-screen keyboard";
          PartOf = "graphical-session.target";
          After = "graphical-session.target";
        };
        Service = {
          ExecStart = "${getExe' pkgs.wvkbd "wvkbd-mobintl"} --auto --hidden -L 300 --landscape-layers pc,pcru";
          Restart = "on-failure";
          Slice = "app.slice";
        };
      };

      wvkbd-hardware-keyboard = {
        Unit = {
          Description = "Hide the on-screen keyboard while a hardware keyboard is attached";
          PartOf = "graphical-session.target";
          After = "graphical-session.target";
        };
        Service = {
          ExecStart = getExe wvkbdHardwareKeyboard;
          Restart = "on-failure";
          Slice = "app.slice";
        };
        Install.WantedBy = [ "graphical-session.target" ];
      };

      lisgd = mkIf (cfg.touchscreen != null) {
        Unit = {
          Description = "Touchscreen gestures";
          PartOf = "graphical-session.target";
          After = "graphical-session.target";
        };
        Service = {
          ExecStart = lib.escapeShellArgs (
            [
              (getExe pkgs.lisgd)
              "-d"
              cfg.touchscreen
            ]
            ++ lib.concatMap (g: [
              "-g"
              g
            ]) gestures
          );
          Restart = "on-failure";
          Slice = "app.slice";
        };
        Install.WantedBy = [ "graphical-session.target" ];
      };
    };
  };
}
