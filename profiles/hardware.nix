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
    config.pipewire = {
      "context.properties" = {
        "default.clock.quantum" = 2048;
        "default.clock.min-quantum" = 1024;
        "default.clock.max-quantum" = 4096;
      };
    };
    # jack.enable = true;
  };

  hardware = {
    cpu = {
      amd.updateMicrocode = lib.mkIf (config.device == "AMD-Desktop" || config.device == "Honor-MB-AMD-Laptop") true;
      intel.updateMicrocode = lib.mkIf (config.device == "Fujitsu-AH531-Laptop") true;
    };

    opengl = {
      enable = true;
      driSupport32Bit = true;
    };

    bluetooth = {
      # TODO: Fix state after persist
      enable = lib.mkIf config.deviceSpecific.isLaptop true;
      # For battery provider, bluezFull is just an alias for bluez
      package = pkgs.bluez5-experimental;
      settings.General.Experimental = true;
      # hsphfpd.enable = true;
      # powerOnBoot = false;
    };
  };

  # Speed up boot
  # https://www.freedesktop.org/software/systemd/man/systemd-udev-settle.service.html
  systemd.services.systemd-udev-settle.enable = false;
  systemd.services.NetworkManager-wait-online.enable = false;

  # Disable suspend on lid switch on laptop
  services.logind.lidSwitch = lib.mkIf config.deviceSpecific.isLaptop "ignore";

  # Use BFQ for HDD drives on desktop
  # TODO: Rewrite this because it is ugly
  # boot.kernelParams = lib.mkIf config.deviceSpecific.isDesktop [ "scsi_mod.use_blk_mq=1" ];
  # services.udev.extraRules = lib.mkIf config.deviceSpecific.isDesktop ''
  #   ACTION=="add|change", KERNEL=="sd[a-z]", ATTR{queue/rotational}=="1", ATTR{queue/scheduler}="bfq"
  # '';
}
