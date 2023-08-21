{ pkgs, config, lib, ... }:
{
  boot.extraModprobeConfig = lib.mkIf (config.device == "Fujitsu-AH531-Laptop") ''
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

  services.udev.extraHwdb = lib.mkIf (config.device == "INFINITY") ''
    evdev:name:Huawei WMI hotkeys:*
      KEYBOARD_KEY_287=f20
  '';

  hardware = {
    cpu = {
      amd.updateMicrocode = lib.mkIf (config.device == "AMD-Desktop" || config.device == "INFINITY") true;
      intel.updateMicrocode = lib.mkIf (config.device == "Fujitsu-AH531-Laptop") true;
    };

    opengl = {
      enable = true;
      driSupport32Bit = true;

      # Work-around for LibreOffice to get OpenCL work
      setLdLibraryPath = true;
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

  # Speed up boot
  # https://www.freedesktop.org/software/systemd/man/systemd-udev-settle.service.html
  systemd.services.systemd-udev-settle.enable = false;
  systemd.services.NetworkManager-wait-online.enable = !config.deviceSpecific.isLaptop;

  # Disable suspend on lid switch on laptop
  services.logind.lidSwitch = lib.mkIf (config.deviceSpecific.isLaptop) "ignore";
}
