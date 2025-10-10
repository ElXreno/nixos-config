{
  config,
  namespace,
  lib,
  ...
}:
let
  inherit (lib) mkIf mkEnableOption;
  cfg = config.${namespace}.system.hardware.huawei-wmi;
in
{
  options.${namespace}.system.hardware.huawei-wmi = {
    enable = mkEnableOption "Whether or not to manage huawei wmi.";
  };

  config = mkIf cfg.enable {
    services.udev = {
      extraHwdb = ''
        evdev:name:Huawei WMI hotkeys:*
          KEYBOARD_KEY_287=f20
      '';
    };
  };
}
