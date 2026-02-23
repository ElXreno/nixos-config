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
in
{
  options.${namespace}.system.zswap = {
    enable = mkEnableOption "Whether or not to manage zswap.";
    swapfilePath = mkOption {
      type = types.str;
      default = concatStrings [
        (lib.optionalString config.${namespace}.system.impermanence.enable "/mnt/root-persist")
        "/swapfile"
      ];
    };
    swapfileSize = mkOption {
      type = types.int;
      default = 16 * 1024; # 16 GB
    };
    createSwapfile = mkEnableOption "Whether to create a swapfile." // {
      default = true;
    };
  };

  config = mkIf cfg.enable {
    assertions = [
      {
        assertion = !(cfg.enable && config.${namespace}.system.zram.enable);
        message = ''
          Disallowed use of `${namespace}.system.zswap.enable` and `${namespace}.system.zram.enable` simultaneously.
        '';
      }
    ];

    swapDevices = mkIf cfg.createSwapfile [
      {
        device = cfg.swapfilePath;
        size = cfg.swapfileSize;
      }
    ];

    boot.kernelParams = [
      "zswap.enabled=1"
      "zswap.compressor=zstd" # It already used by default
      "zswap.max_pool_percent=20"
      "zswap.shrinker_enabled=1"
    ];

    boot.kernel.sysctl = {
      "vm.page-cluster" = 0;
      "vm.swappiness" = 100;
    };
  };
}
