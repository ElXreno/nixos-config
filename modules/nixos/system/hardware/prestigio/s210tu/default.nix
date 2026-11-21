{
  config,
  namespace,
  lib,
  ...
}:
let
  inherit (lib) mkIf mkEnableOption;
  cfg = config.${namespace}.system.hardware.prestigio.s210tu;
in
{
  options.${namespace}.system.hardware.prestigio.s210tu = {
    enable = mkEnableOption "managing Prestigio S210TU stuff" // {
      default = config.${namespace}.facts.system.board == "S210TU";
      defaultText = "Whether the SMBIOS board is S210TU.";
    };
  };

  config = mkIf cfg.enable {
    services.udev.extraHwdb = ''
      evdev:atkbd:dmi:*:svnPrestigio:pnMultipadVisconte2:*
        KEYBOARD_KEY_db=prog1
    '';
  };
}
