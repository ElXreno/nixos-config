{
  config,
  namespace,
  lib,
  ...
}:
let
  inherit (lib) mkIf mkEnableOption;
  cfg = config.${namespace}.system.hardware.honor.bmh-wcx9;
in
{
  options.${namespace}.system.hardware.honor.bmh-wcx9 = {
    enable = mkEnableOption "managing HONOR BMH-WCX9 stuff" // {
      default = config.${namespace}.facts.system.board == "BMH-WCX9-PCB";
      defaultText = "Whether the SMBIOS board is BMH-WCX9-PCB.";
    };
  };

  config = mkIf cfg.enable {
    boot.kernelParams = [
      "amdgpu.dcdebugmask=0x10"
      "amdgpu.sg_display=0"
      "amdgpu.hdmi6g=1"
      "tsc=reliable"
      "clocksource=tsc"
    ];
  };
}
