{
  config,
  namespace,
  lib,
  ...
}:

let
  inherit (lib) mkDefault mkIf mkEnableOption;
  cfg = config.${namespace}.roles.common;
in
{
  options.${namespace}.roles.common = {
    enable = mkEnableOption "common configuration";
  };

  config = mkIf cfg.enable {
    ${namespace} = {
      system.boot.enable = mkDefault true;

      services = {
        dnscrypt-proxy.enable = true;
        openssh.enable = true;
      };

      programs = {
        fish.enable = true;
      };

      system.tools.enable = true;
      system.zswap.enable = mkDefault true;
    };

    services.irqbalance.enable = true;
  };
}
