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
    enable = mkEnableOption "managing huawei wmi" // {
      default = lib.elem config.${namespace}.facts.system.vendor [
        "HUAWEI"
        "HONOR"
      ];
      defaultText = "Whether the SMBIOS vendor is HUAWEI or HONOR.";
    };
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
