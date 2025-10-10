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
    ;
  cfg = config.${namespace}.services.xserver;
in
{
  options.${namespace}.services.xserver = {
    enable = mkEnableOption "Whether or not to manage xserver.";
    isLaptop = mkEnableOption "Whether to enable libinput." // {
      default = true;
    };
  };

  config = mkIf cfg.enable {
    services = {
      xserver = {
        enable = true;
        xkb = {
          layout = "us,ru";
          options = "grp:alt_shift_toggle";
        };
      };

      libinput = mkIf cfg.isLaptop {
        enable = true;
        mouse.accelSpeed = "-0.4";
        touchpad = {
          tappingDragLock = false;
          naturalScrolling = true;
        };
      };
    };
  };
}
