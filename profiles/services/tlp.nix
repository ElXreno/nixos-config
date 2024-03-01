{
  services = {
    tlp = {
      enable = true;
      settings = {
        CPU_ENERGY_PERF_POLICY_ON_AC = "performance";
        CPU_ENERGY_PERF_POLICY_ON_BAT = "balance_power";
        PCIE_ASPM_ON_BAT = "powersupersave";
        USB_BLACKLIST = "046d:c542"; # Logitech M190
      };
    };
  };

  # Plasma will use power-profiles-daemon which conflicts with tlp
  # https://github.com/NixOS/nixpkgs/pull/175738
  # But I don't care
  services.power-profiles-daemon.enable = false;
}
