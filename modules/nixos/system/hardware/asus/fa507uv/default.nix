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
    mkMerge
    ;
  cfg = config.${namespace}.system.hardware.asus.fa507uv;
in
{
  options.${namespace}.system.hardware.asus.fa507uv = {
    enable = mkEnableOption "managing ASUS FA507UV stuff" // {
      default = config.${namespace}.facts.system.board == "FA507UV";
      defaultText = "Whether the SMBIOS board is FA507UV.";
    };
  };

  config = mkIf cfg.enable (mkMerge [
    {
      ${namespace} = {
        services = {
          asusd.enable = true;
          supergfxd.enable = true;
          pipewire.rnnoise.target = "alsa_input.pci-0000_66_00.6.analog-stereo";
        };

        system.hardware = {
          cpu.amd.freq-floor.enable = true;
          gpu.amd.dpm = {
            enable = true;
            acLevel = "auto";
          };
          gpu.nvidia.dynamicBoost.enable = true;
        };
      };

      hardware.cpu.amd.ryzen-smu.enable = true;
    }

    {
      services.udev.extraRules = ''
        # Samsung 9100 PRO
        ACTION=="add", SUBSYSTEM=="pci", ATTR{class}=="0x010802", ATTR{power/control}="auto"
      '';
    }

    {
      boot.kernelParams = [
        "amdgpu.dcdebugmask=0x8"
        "acpi_osi=!"
        "acpi_osi=\"Windows 2022\""
        "tsc=nowatchdog"
        "tsc=reliable"
        "clocksource=tsc"
        "pcie_aspm.policy=powersupersave"
      ];
    }
  ]);
}
