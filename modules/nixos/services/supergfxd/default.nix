{
  config,
  namespace,
  lib,
  ...
}:

let
  inherit (lib) mkIf mkEnableOption;
  cfg = config.${namespace}.services.supergfxd;
in
{
  options.${namespace}.services.supergfxd = {
    enable = mkEnableOption "Whether or not to manage supergfxd.";
  };

  config = mkIf cfg.enable {
    services.supergfxd = {
      enable = true;
      settings = {
        mode = "AsusMuxDgpu";
        vfio_enable = false;
        vfio_save = false;
        always_reboot = false;
        no_logind = false;
        logout_timeout_s = 180;
        hotplug_type = "Asus";
      };
    };
  };
}
