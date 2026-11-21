{
  config,
  namespace,
  lib,
  ...
}:
let
  inherit (lib)
    all
    attrValues
    concatLists
    filter
    genAttrs
    hasSuffix
    mapAttrsToList
    mkEnableOption
    mkOption
    optionals
    types
    ;
  cfg = config.${namespace}.system.networking.dns;

  inZone = name: name == cfg.zone || hasSuffix ".${cfg.zone}" name;
  namesOf =
    server:
    optionals server.enable (
      concatLists (
        mapAttrsToList (name: host: [ host.hostName ] ++ host.serverAliases) server.virtualHosts
      )
    );
  virtualHostNames = filter inZone (namesOf config.services.caddy);

  address =
    family:
    mkOption {
      type = types.nullOr types.str;
      default = null;
      description = "Public ${family} address of this host.";
    };
in
{
  options.${namespace}.system.networking.dns = {
    zone = mkOption {
      type = types.str;
      default = "elxreno.com";
      description = "Public DNS zone whose records are built from the hosts.";
    };

    addresses = {
      v4 = address "IPv4";
      v6 = address "IPv6";
    };

    records = mkOption {
      default = { };
      description = ''
        Public names of this host, by fully qualified name. Every caddy
        virtual host inside the zone is one already; set a name here to
        add one or to change how it is published.
      '';
      type = types.attrsOf (
        types.submodule {
          options = {
            cname = mkOption {
              type = types.nullOr types.str;
              default = null;
              description = "Publish the name as a CNAME to this target instead of the addresses of the host.";
            };
            proxied = mkEnableOption "proxying the name through Cloudflare";
          };
        }
      );
    };
  };

  config = {
    ${namespace}.system.networking.dns.records = genAttrs virtualHostNames (_: { });

    assertions = [
      {
        assertion =
          cfg.addresses.v4 != null
          || cfg.addresses.v6 != null
          || all (record: record.cname != null) (attrValues cfg.records);
        message = "${config.networking.hostName} publishes names in ${cfg.zone} but sets no ${namespace}.system.networking.dns.addresses";
      }
    ];
  };
}
