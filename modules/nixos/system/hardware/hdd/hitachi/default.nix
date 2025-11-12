{
  config,
  namespace,
  lib,
  pkgs,
  ...
}:
let
  inherit (lib)
    mkIf
    mkEnableOption
    mkOption
    types
    ;
  cfg = config.${namespace}.system.hardware.hdd.hitachi;
in
{
  options.${namespace}.system.hardware.hdd.hitachi = {
    enable = mkEnableOption "Whether to tune APM and acoustic level on Hitachi HDS721010KLA330.";
    diskPath = mkOption {
      type = types.str;
      default = "/dev/disk/by-id/ata-Hitachi_HDS721010KLA330_GTF002PAKLL4XF";
    };
  };

  config = mkIf cfg.enable {
    systemd.services."hitachi-disk-tuning" = {
      wantedBy = [ "multi-user.target" ];
      after = [ "local-fs.target" ];
      serviceConfig = {
        Type = "oneshot";
        ExecStart = ''
          ${pkgs.hdparm}/sbin/hdparm -B 255 -S 0 -M 254 ${cfg.diskPath}
          ${pkgs.smartmontools}/bin/smartctl -l scterc,70,70 ${cfg.diskPath}
        '';
      };
    };
  };
}
