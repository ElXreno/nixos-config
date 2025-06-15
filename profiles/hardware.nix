{
  pkgs,
  config,
  lib,
  ...
}:
{
  boot.extraModprobeConfig = lib.mkIf config.device.laptop.manufacturer.Fujitsu ''
    options iwlmvm power_scheme=1
    options iwlwifi 11n_disable=8
  '';

  # sound.enable = true;
  security.rtkit.enable = true;
  services.pipewire = {
    enable = true;
    alsa.enable = true;
    alsa.support32Bit = true;
    pulse.enable = true;
  };

  services.udev.extraHwdb = lib.mkIf config.device.laptop.manufacturer.Honor ''
    evdev:name:Huawei WMI hotkeys:*
      KEYBOARD_KEY_287=f20
  '';

  hardware = {
    cpu = {
      amd.updateMicrocode = lib.mkIf config.device.cpu.amd true;
      intel.updateMicrocode = lib.mkIf config.device.cpu.intel true;
    };

    graphics = {
      enable = true;
      enable32Bit = true;
    };

    amdgpu.amdvlk = lib.mkIf config.device.gpu.amd {
      enable = true;
      support32Bit.enable = true;
    };

    bluetooth = {
      # TODO: Fix state after persist
      enable = config.device.network.hasWirelessCard;
      # For battery provider, bluezFull is just an alias for bluez
      package = pkgs.bluez5-experimental;
      settings.General.Experimental = true;
      # powerOnBoot = false;
    };
  };

  # Speed up boot
  # https://www.freedesktop.org/software/systemd/man/systemd-udev-settle.service.html
  # systemd.services.systemd-udev-settle.enable = false;
  # systemd.services.NetworkManager-wait-online.enable = (config.device.network.wirelessCard != null);

  # Disable suspend on lid switch
  services.logind.lidSwitch = "ignore";
}
