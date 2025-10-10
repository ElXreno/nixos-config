{
  config,
  namespace,
  lib,
  ...
}:

let
  inherit (lib) mkIf mkEnableOption;
  cfg = config.${namespace}.roles.common;
in
{
  options.${namespace}.roles.common = {
    enable = mkEnableOption "Whether or not to enable common configuration.";
  };

  config = mkIf cfg.enable {
    ${namespace} = {
      system.boot.enable = true;

      services = {
        dnscrypt-proxy.enable = true;
        irqbalance.enable = true;
        openssh.enable = true;
        tailscale.enable = true;
      };

      programs = {
        fish.enable = true;
      };

      system.zram.enable = true;

      sops.enable = true;
    };
  };
}
