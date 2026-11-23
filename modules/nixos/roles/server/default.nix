{
  config,
  namespace,
  lib,
  ...
}:

let
  inherit (lib) mkIf mkDefault mkEnableOption;
  cfg = config.${namespace}.roles.server;
in
{
  options.${namespace}.roles.server = {
    enable = mkEnableOption "server configuration";
  };

  config = mkIf cfg.enable {
    ${namespace} = {
      roles.common.enable = true;

      tailscale.tags = [ "tag:server" ];

      system = {
        nix = {
          auto-optimise.enable = true;
          gc.enable = true;
        };

        boot.interactive = mkDefault false;
        tuning = {
          profile = mkDefault "server";
          sched.enable = mkDefault true;
        };

        networking = {
          networkmanager.enable = mkDefault false;
          waitOnline = mkDefault true;
        };

        documentation.enable = false;
      };
    };

    services.resolved.settings.Resolve = {
      LLMNR = false;
      MulticastDNS = false;
    };
  };
}
