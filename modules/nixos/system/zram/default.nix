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
    ;
  cfg = config.${namespace}.system.zram;
in
{
  options.${namespace}.system.zram = {
    enable = mkEnableOption "Whether or not to manage zram.";
  };

  config = mkIf cfg.enable {
    zramSwap = {
      enable = true;
      algorithm = "zstd";
      memoryPercent = 300;
    };

    boot.kernel.sysctl = {
      "vm.page-cluster" = 0;
      "vm.swappiness" = 80;
    };
  };
}
