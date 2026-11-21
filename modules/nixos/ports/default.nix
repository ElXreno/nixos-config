{
  config,
  namespace,
  lib,
  ...
}:

let
  inherit (lib)
    attrNames
    concatStringsSep
    filterAttrs
    groupBy
    length
    mapAttrsToList
    mkOption
    types
    ;
  cfg = config.${namespace}.ports;
  byValue = groupBy (name: toString cfg.${name}) (attrNames cfg);
  collisions = filterAttrs (_: names: length names > 1) byValue;
in
{
  options.${namespace}.ports = mkOption {
    type = types.attrsOf types.port;
    default = { };
    description = "Registry of ports in use on this host.";
  };

  config.assertions = [
    {
      assertion = collisions == { };
      message = "Port collisions on ${config.networking.hostName}: ${
        concatStringsSep "; " (
          mapAttrsToList (port: names: "${port} used by ${concatStringsSep ", " names}") collisions
        )
      }";
    }
  ];
}
