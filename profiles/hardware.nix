{
  pkgs,
  config,
  lib,
  ...
}:
{
  security.rtkit.enable = true;
  services.pipewire = {
    enable = true;
    alsa.enable = true;
    alsa.support32Bit = true;
    pulse.enable = true;
  };

  services.udev.extraHwdb = lib.mkIf (config.device == "INFINITY") ''
    evdev:name:Huawei WMI hotkeys:*
      KEYBOARD_KEY_287=f20
  '';

  hardware = {
    cpu = {
      amd.updateMicrocode = lib.mkIf (
        config.device == "AMD-Desktop" || config.device == "INFINITY" || config.device == "KURWA"
      ) true;
    };

    graphics = {
      enable = true;
      enable32Bit = true;
    };

    amdgpu.amdvlk = lib.mkIf (config.device == "INFINITY") {
      enable = true;
      support32Bit.enable = true;
    };

    bluetooth = {
      # TODO: Fix state after persist
      enable = lib.mkIf config.deviceSpecific.isLaptop true;
      # For battery provider, bluezFull is just an alias for bluez
      package = pkgs.bluez5-experimental;
      settings.General.Experimental = true;
      # powerOnBoot = false;
    };
  };

  # Disable suspend on lid switch
  services.logind.lidSwitch = "ignore";
}
