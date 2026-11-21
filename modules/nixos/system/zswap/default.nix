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
    concatStrings
    ;
  cfg = config.${namespace}.system.zswap;
  impermanence = config.${namespace}.system.impermanence;

  diskoSwap = lib.any (
    disk:
    lib.any (partition: (partition.content.type or null) == "swap") (
      lib.attrValues (disk.content.partitions or { })
    )
  ) (lib.attrValues config.disko.devices.disk);
in
{
  options.${namespace}.system.zswap = {
    enable = mkEnableOption "managing zswap";
    swapfilePath = mkOption {
      type = types.str;
      default = concatStrings [
        (lib.optionalString impermanence.enable impermanence.defaultPersistentPath)
        "/swapfile"
      ];
    };
    swapfileSize = mkOption {
      type = types.int;
      default =
        let
          ram = config.${namespace}.facts.memory.totalMiB;
          roundUp = size: if size >= ram then size else roundUp (size * 2);
        in
        lib.min (16 * 1024) (roundUp 1024);
      defaultText = "RAM size from facts rounded up to a power of two, at most 16 GiB.";
      description = "Swapfile size in MiB.";
    };
    createSwapfile = mkEnableOption "creating a swapfile" // {
      default = !diskoSwap;
      defaultText = "Whether disko declares no swap partition.";
    };
  };

  config = mkIf cfg.enable {
    swapDevices = mkIf cfg.createSwapfile [
      {
        device = cfg.swapfilePath;
        size = cfg.swapfileSize;
      }
    ];

    boot.kernelParams = [
      "zswap.enabled=1"
      "zswap.max_pool_percent=20"
      "zswap.shrinker_enabled=1"
    ];

    boot.kernel.sysctl = {
      "vm.swappiness" = 100;
    };
  };
}
