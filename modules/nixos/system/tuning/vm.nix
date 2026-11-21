{
  config,
  namespace,
  lib,
  ...
}:
let
  inherit (lib) mkIf mkMerge;
  inherit (config.${namespace}.system.tuning) profile;

  isServer = profile == "server";
  isLaptop = profile == "laptop";
  isXanmod = config.${namespace}.system.boot.kernel.flavour == "xanmod";
in
{
  boot.kernel.sysctl = mkMerge [
    {
      "vm.compaction_proactiveness" = 40;
      "vm.min_free_kbytes" = lib.min 262144 (config.${namespace}.facts.memory.totalMiB * 1024 / 24);
      "vm.vfs_cache_pressure" = if isServer then 500 else 3000;
    }
    (mkIf (!isServer) {
      "kernel.sysrq" = 1;
      "vm.extfrag_threshold" = 300;
    })
    # reclaim stalls under the clean file floor once most of RAM is pinned
    (mkIf isXanmod {
      "vm.workingset_protection" = 0;
    })
    (mkIf isLaptop {
      "vm.dirty_writeback_centisecs" = 1500;
    })
  ];

  systemd.tmpfiles.rules = [
    "w- /sys/kernel/mm/transparent_hugepage/defrag - - - - defer"
  ];
}
