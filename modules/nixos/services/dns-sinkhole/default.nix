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
  cfg = config.${namespace}.services.dns-sinkhole;
in
{
  options.${namespace}.services.dns-sinkhole = {
    enable = mkEnableOption "Whether to enable the LAN DNS sinkhole.";

    interface = mkOption {
      type = types.str;
      description = "LAN interface to bind the resolver on.";
    };

    upstream = mkOption {
      type = types.str;
      default = "127.0.0.1#53";
      description = "Upstream resolver for non-sinkholed queries.";
    };

    nxdomainDomains = mkOption {
      type = types.listOf types.str;
      default = [ "io.mi.com" ];
      description = "Domain subtrees answered with NXDOMAIN.";
    };
  };

  config = mkIf cfg.enable {
    services.dnsmasq = {
      enable = true;
      resolveLocalQueries = false;
      settings = {
        interface = [ cfg.interface ];
        except-interface = [ "lo" ];
        bind-interfaces = true;
        no-resolv = true;
        no-hosts = true;
        server = [ cfg.upstream ];
        local = map (d: "/${d}/") cfg.nxdomainDomains;
        cache-size = 1000;
      };
    };

    networking.firewall.interfaces.${cfg.interface} = {
      allowedUDPPorts = [ 53 ];
      allowedTCPPorts = [ 53 ];
    };
  };
}
