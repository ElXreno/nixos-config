{
  config,
  options,
  namespace,
  lib,
  ...
}:
let
  inherit (builtins) toJSON;
  inherit (lib)
    any
    attrNames
    attrValues
    concatLists
    concatMap
    concatMapStringsSep
    concatStringsSep
    filter
    filterAttrs
    hasSuffix
    isAttrs
    mapAttrs
    mapAttrs'
    mapAttrsToList
    mkIf
    mkOption
    nameValuePair
    optionals
    sort
    types
    unique
    ;
  cfg = config.${namespace}.system.networking.firewall;

  self = "modules/nixos/system/networking/firewall/default.nix";
  kinds = {
    tcp = "allowedTCPPorts";
    udp = "allowedUDPPorts";
    tcpRanges = "allowedTCPPortRanges";
    udpRanges = "allowedUDPPortRanges";
  };

  entries = attrValues cfg.public;
  mirrored = filterAttrs (_: entry: entry.upstream != null) cfg.public;
  acknowledged = mapAttrsToList (_: entry: entry.upstream) mirrored;

  definitions =
    kind:
    filter (
      definition: definition.value != [ ]
    ) options.networking.firewall.${kinds.${kind}}.definitionsWithLocations;
  definedIn = file: definition: hasSuffix file (toString definition.file);
  openedBy =
    file: kind: concatMap (definition: definition.value) (filter (definedIn file) (definitions kind));
  stray = concatMap (
    kind:
    filter (definition: !any (file: definedIn file definition) ([ self ] ++ acknowledged)) (
      definitions kind
    )
  ) (attrNames kinds);

  rank = port: if isAttrs port then port.from * 65536 + port.to else port;
  canonical = ports: sort (a: b: rank a < rank b) (unique ports);
  drifted = concatLists (
    mapAttrsToList (
      name: entry:
      concatMap (
        kind:
        let
          opened = canonical (openedBy entry.upstream kind);
        in
        optionals (opened != canonical entry.${kind}) [
          "${entry.upstream} opens ${kind} ${toJSON opened}, public.${name} declares ${
            toJSON (canonical entry.${kind})
          }"
        ]
      ) (attrNames kinds)
    ) mirrored
  );

  portRange = types.attrsOf types.port;
  portList =
    type: description:
    mkOption {
      type = types.listOf type;
      default = [ ];
      inherit description;
    };
in
{
  options.${namespace}.system.networking.firewall = {
    public = mkOption {
      default = { };
      description = ''
        Everything this host accepts on every interface, by name. The only
        way to open a port outside a single interface: an opening defined
        anywhere else fails the build.
      '';
      type = types.attrsOf (
        types.submodule {
          options = {
            tcp = portList types.port "TCP ports to open.";
            udp = portList types.port "UDP ports to open.";
            tcpRanges = portList portRange "TCP port ranges to open.";
            udpRanges = portList portRange "UDP port ranges to open.";
            upstream = mkOption {
              type = types.nullOr types.str;
              default = null;
              example = "nixos/modules/programs/steam.nix";
              description = ''
                Path suffix of a module outside this repository that opens
                these ports itself. The entry then opens nothing and the
                build fails unless that module opens exactly what is
                declared here.
              '';
            };
          };
        }
      );
    };

    exposed = mkOption {
      readOnly = true;
      description = "The ports of every {option}`public` entry that opens any on this host.";
      default = filterAttrs (_: opened: opened != { }) (
        mapAttrs (
          _: entry: filterAttrs (_: ports: ports != [ ]) (mapAttrs (kind: _: canonical entry.${kind}) kinds)
        ) cfg.public
      );
    };
  };

  config = {
    ${namespace}.system.networking.firewall.public = {
      mdns = {
        upstream = "nixosModules/clanCore/networking.nix";
        udp = [ 5353 ];
      };
      avahi = mkIf config.services.avahi.enable {
        upstream = "nixos/modules/services/networking/avahi-daemon.nix";
        udp = [ 5353 ];
      };
    };

    networking.firewall = mapAttrs' (
      kind: option:
      nameValuePair option (concatMap (entry: optionals (entry.upstream == null) entry.${kind}) entries)
    ) kinds;

    assertions = [
      {
        assertion = stray == [ ];
        message = "Ports opened on ${config.networking.hostName} outside ${namespace}.system.networking.firewall.public: ${
          concatMapStringsSep "; " (
            definition: "${toString definition.file} opens ${toJSON definition.value}"
          ) stray
        }";
      }
      {
        assertion = drifted == [ ];
        message = "Ports opened on ${config.networking.hostName} differ from ${namespace}.system.networking.firewall.public: ${concatStringsSep "; " drifted}";
      }
    ];
  };
}
