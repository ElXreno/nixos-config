{
  config,
  namespace,
  lib,
  ...
}:

let
  inherit (lib) mkIf mkEnableOption;
  cfg = config.${namespace}.services.bitmagnet;
in
{
  options.${namespace}.services.bitmagnet = {
    enable = mkEnableOption "Whether or not to manage bitmagnet.";
    autostart = mkEnableOption "Whether to execute sing-box client at system boot.";
  };

  config = mkIf cfg.enable {
    services.bitmagnet = {
      enable = true;
      openFirewall = true;
      settings = {
        tmdb.enabled = false;
        processor.concurrency = 16;
        dht_crawler.scaling_factor = 50;
        dht_crawler.save_files_threshold = 1000;
      };
    };

    systemd.services.bitmagnet = lib.mkIf (!cfg.autostart) {
      wantedBy = lib.mkForce [ ];
    };
  };
}
