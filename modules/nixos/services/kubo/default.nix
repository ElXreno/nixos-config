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
    optionalAttrs
    types
    ;
  cfg = config.${namespace}.services.kubo;
  ports = config.${namespace}.ports;
in
{
  options.${namespace}.services.kubo = {
    enable = mkEnableOption "kubo";
    forwardedPort = mkOption {
      type = types.nullOr types.port;
      default = null;
      description = "Swarm forwarded port.";
    };
  };

  config = mkIf cfg.enable {
    ${namespace} = {
      ports = {
        kubo-api = 5001;
        kubo-gateway = 8082;
      }
      // optionalAttrs (cfg.forwardedPort != null) { kubo-swarm = cfg.forwardedPort; };
      system.networking.routes.confine.ipfs = "airvpn";
      system.impermanence.directories = [
        {
          directory = "/var/lib/ipfs";
          user = "ipfs";
          group = "ipfs";
        }
      ];
    };

    services.kubo = {
      enable = true;
      enableGC = true;
      startWhenNeeded = true;
      settings = {
        Addresses.API = "/ip4/127.0.0.1/tcp/${toString ports.kubo-api}";
        Addresses.Gateway = "/ip4/127.0.0.1/tcp/${toString ports.kubo-gateway}";
        AutoTLS.Enabled = true;
        Datastore = {
          StorageMax = "100GB";
          BloomFilterSize = 1199120;
        };
        Internal.Bitswap.ProviderSearchDelay = "0s";
        Provide.Strategy = "all";
        Routing.AcceleratedDHTClient = true;
        Swarm.ConnMgr = {
          LowWater = 100;
          HighWater = 300;
        };
        Addresses.Swarm = mkIf (cfg.forwardedPort != null) [
          "/ip4/0.0.0.0/tcp/${toString cfg.forwardedPort}"
          "/ip6/::/tcp/${toString cfg.forwardedPort}"
          "/ip4/0.0.0.0/udp/${toString cfg.forwardedPort}/quic-v1"
          "/ip4/0.0.0.0/udp/${toString cfg.forwardedPort}/quic-v1/webtransport"
          "/ip6/::/udp/${toString cfg.forwardedPort}/quic-v1"
          "/ip6/::/udp/${toString cfg.forwardedPort}/quic-v1/webtransport"
        ];
      };
    };

    vpnNamespaces.airvpn.openVPNPorts = mkIf (cfg.forwardedPort != null) [
      {
        port = cfg.forwardedPort;
        protocol = "both";
      }
    ];

    users.users.${config.${namespace}.primaryUser}.extraGroups = [ config.services.kubo.group ];
  };
}
