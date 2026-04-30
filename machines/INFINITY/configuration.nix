{
  namespace,
  ...
}:

{
  ${namespace} = {
    roles = {
      laptop.enable = true;
    };

    system = {
      boot = {
        uefi.enable = true;
        kernel.optimizations.enable = true;
      };
      impermanence.enable = true;
      hardware = {

        huawei-wmi.enable = true;
      };

      nix.gc.enable = true;
    };

    desktop-environments.plasma.enable = true;

    home-manager.syncthing.randomPortIncrement = 66;
  };

  boot.kernelParams = [
    "amdgpu.dcdebugmask=0x10"
    "amdgpu.sg_display=0"
    "tsc=reliable"
    "clocksource=tsc"
  ];

  i18n.defaultLocale = "ru_RU.UTF-8";
}
