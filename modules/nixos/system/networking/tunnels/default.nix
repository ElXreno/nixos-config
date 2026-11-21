{
  config,
  namespace,
  lib,
  ...
}:

let
  inherit (lib)
    attrNames
    attrValues
    concatMapStringsSep
    elem
    filterAttrs
    genAttrs
    mapAttrs
    mapAttrs'
    mkDefault
    mkEnableOption
    mkIf
    mkMerge
    mkOption
    nameValuePair
    types
    unique
    ;
  cfg = config.${namespace}.system.networking;
  routed = unique (
    cfg.routes.cgroups ++ attrNames (filterAttrs (_: entries: entries != [ ]) cfg.routes.domains)
  );
  referenced = unique (attrValues cfg.routes.confine ++ routed);
  inherit (config.clan.core.vars) generators;
  enabled = filterAttrs (_: tunnel: tunnel.enable && tunnel.type == "wireguard") cfg.tunnels;
  hostAccess = filterAttrs (_: tunnel: tunnel.hostAccess.enable) enabled;
  position = name: names: lib.lists.findFirstIndex (other: other == name) null names;
  index = name: position name (attrNames cfg.tunnels);

  tunnelModule =
    { name, config, ... }:
    {
      options = {
        enable = mkEnableOption "the tunnel" // {
          default = elem name referenced;
          defaultText = "Whether routes.confine, routes.cgroups or routes.domains refers to the tunnel.";
        };
        type = mkOption {
          type = types.enum [
            "wireguard"
            "hysteria2"
          ];
          default = "wireguard";
          description = "Tunnel protocol: wireguard lives in a network namespace, hysteria2 is an outbound of the domain router.";
        };
        server = mkOption {
          type = types.str;
          default = "";
          description = "Server host of a hysteria2 tunnel.";
        };
        port = mkOption {
          type = types.port;
          default = 443;
          description = "Server port of a hysteria2 tunnel.";
        };
        passwordFile = mkOption {
          type = types.str;
          default = "";
          description = "File holding the password of a hysteria2 tunnel.";
        };
        generator = {
          name = mkOption {
            type = types.str;
            default = "wg-${name}";
            description = "Clan vars generator holding the wg-quick config.";
          };
          file = mkOption {
            type = types.str;
            default = "conf";
            description = "File of the generator holding the wg-quick config.";
          };
          description = mkOption {
            type = types.str;
            default = "${name} wg-quick config.";
            description = "Prompt shown when the config is asked for.";
          };
        };
        configFile = mkOption {
          type = types.str;
          default = generators.${config.generator.name}.files.${config.generator.file}.path;
          defaultText = "Path of the generator file.";
          description = "The wg-quick config of the tunnel.";
        };
        mtu = mkOption {
          type = types.nullOr types.ints.positive;
          default = null;
          description = "Tunnel MTU.";
        };
        endpoints = mkOption {
          type = types.listOf types.str;
          default = [ ];
          description = "Server endpoints of a wireguard tunnel, one interface each on the keys of the config; empty keeps the endpoint of the config.";
        };
        hostAccess = {
          enable = mkEnableOption "routing marked host traffic into the tunnel" // {
            default = config.type == "wireguard" && elem name routed;
            defaultText = "Whether routes.cgroups or routes.domains refers to this wireguard tunnel.";
          };
          mark = mkOption {
            type = types.ints.positive;
            default = 16385 + index name;
            defaultText = "0x4001 plus the position of the tunnel among all declared ones.";
            readOnly = true;
            description = "Firewall mark that sends host traffic into the tunnel.";
          };
          table = mkOption {
            type = types.ints.positive;
            default = 4001 + index name;
            defaultText = "4001 plus the position of the tunnel among all declared ones.";
            readOnly = true;
            description = "Routing table and rule priority used for marked host traffic.";
          };
        };
      };
    };
in
{
  options.${namespace}.system.networking = {
    tunnels = mkOption {
      type = types.attrsOf (types.submodule tunnelModule);
      default = { };
      description = "VPN tunnels, declared once and referred to by name.";
    };
    tunnelTransportMark = mkOption {
      type = types.ints.positive;
      default = 16640;
      readOnly = true;
      description = "Firewall mark on the encrypted packets of every WireGuard tunnel.";
    };
    routes.confine = mkOption {
      type = types.attrsOf types.str;
      default = { };
      example = {
        roj-crawler = "airvpn";
      };
      description = "Systemd services confined to the network namespace of a tunnel.";
    };
  };

  config = {
    assertions = map (tunnel: {
      assertion = cfg.tunnels ? ${tunnel} && cfg.tunnels.${tunnel}.enable;
      message = "routes refer to the unknown or disabled tunnel ${tunnel}.";
    }) referenced;

    clan.core.vars.generators = mapAttrs' (
      _: tunnel:
      nameValuePair tunnel.generator.name {
        prompts.${tunnel.generator.file} = {
          inherit (tunnel.generator) description;
          type = "multiline-hidden";
          persist = true;
        };
      }
    ) enabled;

    networking.firewall.extraReversePathFilterRules = concatMapStringsSep "\n" (
      name: ''iifname "${name}-br" accept''
    ) (attrNames hostAccess);

    systemd.network.config.networkConfig.ManageForeignRoutingPolicyRules = mkIf (
      hostAccess != { }
    ) false;

    vpnNamespaces = mapAttrs (
      name: tunnel:
      let
        offset = position name (attrNames enabled);
        v4 = "192.168.${toString (15 + offset)}";
        v6 = "fd93:9701:1d0${toString offset}:";
      in
      {
        enable = true;
        wireguardConfigFile = tunnel.configFile;
        inherit (tunnel) mtu endpoints;
        transportMark = cfg.tunnelTransportMark;
        hostAccess = {
          inherit (tunnel.hostAccess) enable mark table;
        };
        namespaceAddress = mkDefault "${v4}.1";
        bridgeAddress = mkDefault "${v4}.5";
        namespaceAddressIPv6 = mkDefault "${v6}:2";
        bridgeAddressIPv6 = mkDefault "${v6}:1";
      }
    ) enabled;

    systemd.services = mkMerge [
      (genAttrs (attrNames hostAccess) (_: {
        wantedBy = [ "multi-user.target" ];
      }))
      (mapAttrs (_: tunnel: {
        vpnConfinement = {
          enable = true;
          vpnNamespace = tunnel;
        };
        partOf = [ "${tunnel}.service" ];
      }) cfg.routes.confine)
    ];
  };
}
