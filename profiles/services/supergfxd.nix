{ pkgs, ... }:
{
  services.supergfxd = {
    enable = true;
    settings = {
      mode = "Hybrid";
      vfio_enable = false;
      vfio_save = false;
      always_reboot = false;
      no_logind = false;
      logout_timeout_s = 180;
      hotplug_type = "Asus";
    };
  };
}
