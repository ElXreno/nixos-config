{
  config,
  namespace,
  lib,
  ...
}:
let
  inherit (lib)
    mkIf
    mkEnableOption
    mkOption
    types
    ;
  cfg = config.${namespace}.system.fs.zfs;
in
{
  options.${namespace}.system.fs.zfs = {
    enable = mkEnableOption "Whether or not to manage zfs.";
    hostId = mkOption {
      type = types.str;
      default = null;
    };
  };

  config = mkIf cfg.enable {
    boot.initrd.supportedFilesystems = [ "zfs" ];
    boot.supportedFilesystems = [ "zfs" ];

    boot.kernelParams = [
      "init_on_alloc=0"
      "zfs.metaslab_lba_weighting_enabled=0"
      "zfs.zfs_arc_sys_free=1073741824"
    ];

    networking.hostId = cfg.hostId;
  };
}
