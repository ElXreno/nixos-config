{
  config,
  lib,
  pkgs,
  ...
}:

let
  inherit (lib)
    concatMap
    concatMapStringsSep
    concatStringsSep
    escapeShellArgs
    filter
    filterAttrs
    genList
    hasInfix
    head
    imap1
    length
    mapAttrs
    max
    mkEnableOption
    mkIf
    mkOption
    optionalString
    types
    unique
    ;

  namespaces = filterAttrs (_: namespace: namespace.enable) config.vpnNamespaces;
  ipv6 = config.networking.enableIPv6;
  isV4 = address: !hasInfix ":" address;
  lines = concatStringsSep "\n";

  protocols =
    protocol:
    if protocol == "both" then
      [
        "tcp"
        "udp"
      ]
    else
      [ protocol ];
  portMatchesBy =
    direction: attribute: entries:
    concatMap (
      entry:
      map (protocol: "${protocol} ${direction} ${toString entry.${attribute}}") (protocols entry.protocol)
    ) entries;
  portMatches = portMatchesBy "dport";

  portModule = attributes: {
    options =
      mapAttrs (
        _: description:
        mkOption {
          type = types.port;
          inherit description;
        }
      ) attributes
      // {
        protocol = mkOption {
          type = types.enum [
            "tcp"
            "udp"
            "both"
          ];
          default = "tcp";
          description = "Transport protocol of the port.";
        };
      };
  };

  namespaceModule =
    { name, config, ... }:
    {
      options = {
        enable = mkEnableOption "the network namespace";
        wireguardConfigFile = mkOption {
          type = types.path;
          description = "The wg-quick config the namespace is built from; it must set DNS.";
        };
        mtu = mkOption {
          type = types.nullOr types.ints.positive;
          default = null;
          description = "MTU of the WireGuard interface, the config value or 1420 when unset.";
        };
        transportMark = mkOption {
          type = types.nullOr types.ints.positive;
          default = null;
          description = "Firewall mark on the encrypted packets, which leave from the host namespace.";
        };
        hostAccess = {
          enable = mkEnableOption "routing marked host traffic into the namespace";
          mark = mkOption {
            type = types.ints.positive;
            description = "Firewall mark that sends host traffic into the namespace.";
          };
          table = mkOption {
            type = types.ints.positive;
            description = "Routing table and rule priority used for marked host traffic.";
          };
        };
        accessibleFrom = mkOption {
          type = types.listOf types.str;
          default = [ ];
          description = "Networks outside the namespace that may reach its mapped ports.";
        };
        allowedEgress = mkOption {
          type = types.listOf types.str;
          default = [ ];
          description = "Networks the namespace may open connections to outside the tunnel.";
        };
        namespaceAddress = mkOption {
          type = types.str;
          default = "192.168.15.1";
          description = "IPv4 address of the namespace side of the link to the host.";
        };
        namespaceAddressIPv6 = mkOption {
          type = types.str;
          default = "fd93:9701:1d00::2";
          description = "IPv6 address of the namespace side of the link to the host.";
        };
        bridgeAddress = mkOption {
          type = types.str;
          default = "192.168.15.5";
          description = "IPv4 address of the host side of the link to the namespace.";
        };
        bridgeAddressIPv6 = mkOption {
          type = types.str;
          default = "fd93:9701:1d00::1";
          description = "IPv6 address of the host side of the link to the namespace.";
        };
        openVPNPorts = mkOption {
          type = types.listOf (
            types.submodule (portModule {
              port = "Port accepted on the tunnel interface.";
            })
          );
          default = [ ];
          description = "Ports reachable through the tunnel, for forwarding done by the provider.";
        };
        portMappings = mkOption {
          type = types.listOf (
            types.submodule (portModule {
              from = "Port on the host.";
              to = "Port inside the namespace.";
            })
          );
          default = [ ];
          description = "Host ports forwarded into the namespace.";
        };
        extraRuleset = mkOption {
          type = types.lines;
          default = "";
          description = "nftables ruleset loaded inside the namespace after its own table.";
        };
        endpoints = mkOption {
          type = types.listOf types.str;
          default = [ ];
          description = "Server endpoints, one tunnel each on the keys of the config; empty keeps the endpoint of the config. New flows spread over the tunnels by destination, an incoming TCP connection is answered through the tunnel it came in on, an incoming UDP flow only by a socket bound to that interface.";
        };
        interfaces = mkOption {
          type = types.listOf types.str;
          default = genList (index: "${name}${toString index}") (max 1 (length config.endpoints));
          defaultText = "The namespace name with a number per endpoint.";
          readOnly = true;
          description = "Tunnel interfaces inside the namespace.";
        };
      };
    };

  # one name as is, several as an nftables set
  ifnames =
    namespace:
    if length namespace.interfaces == 1 then
      ''"${head namespace.interfaces}"''
    else
      "{ ${concatMapStringsSep ", " (interface: ''"${interface}"'') namespace.interfaces} }";

  insideRules =
    name: namespace:
    pkgs.writeText "${name}-netns.nft" ''
      table inet confine {
        set dns4 {
          type ipv4_addr
        }
        set dns6 {
          type ipv6_addr
        }
        chain input {
          type filter hook input priority filter; policy drop;
          iifname "lo" accept
          ${optionalString (!namespace.hostAccess.enable) ''
            # conntrack calls the errors of an untracked flow invalid
            icmp type { destination-unreachable, time-exceeded, parameter-problem } accept
            icmpv6 type { destination-unreachable, packet-too-big, time-exceeded, parameter-problem } accept
          ''}
          ct state invalid drop
          ct state established,related accept
          meta l4proto ipv6-icmp accept
          ${lines (
            map (match: ''iifname "veth-${name}" ${match} accept'') (portMatches "to" namespace.portMappings)
          )}
          ${lines (
            map (match: "iifname ${ifnames namespace} ${match} accept") (
              portMatches "port" namespace.openVPNPorts
            )
          )}
        }
        chain forward {
          type filter hook forward priority filter; policy drop;
          ${optionalString namespace.hostAccess.enable ''
            iifname "veth-${name}" oifname ${ifnames namespace} accept
            iifname ${ifnames namespace} oifname "veth-${name}" ct state established,related accept
          ''}
        }
        chain output {
          type filter hook output priority filter; policy accept;
          oifname "veth-${name}" meta l4proto { tcp, udp } th dport 53 drop
          udp dport 53 ip daddr != @dns4 drop
          udp dport 53 ip6 daddr != @dns6 drop
          ${lines (
            map (
              network: ''oifname "veth-${name}" ${if isV4 network then "ip" else "ip6"} daddr ${network} accept''
            ) namespace.allowedEgress
          )}
          oifname "veth-${name}" ct state established,related accept
          oifname "veth-${name}" drop
        }
        chain postrouting {
          type nat hook postrouting priority srcnat; policy accept;
          ${optionalString namespace.hostAccess.enable "oifname ${ifnames namespace} masquerade"}
        }
        # masquerade needs every flow tracked, open ports alone do not
        ${optionalString (!namespace.hostAccess.enable) ''
          chain untracked-in {
            type filter hook prerouting priority raw; policy accept;
            ${lines (
              map (match: "iifname ${ifnames namespace} ${match} notrack") (
                portMatches "port" namespace.openVPNPorts
              )
            )}
          }
          chain untracked-out {
            type filter hook output priority raw; policy accept;
            ${lines (
              map (match: "oifname ${ifnames namespace} ${match} notrack") (
                portMatchesBy "sport" "port" namespace.openVPNPorts
              )
            )}
          }
        ''}
        ${optionalString (length namespace.interfaces > 1) ''
          chain arrived {
            type filter hook prerouting priority mangle; policy accept;
            ${lines (
              imap1 (
                number: interface: ''iifname "${interface}" meta mark set ${toString number}''
              ) namespace.interfaces
            )}
          }
        ''}
      }
      ${namespace.extraRuleset}
    '';

  hostRules =
    name: namespace:
    pkgs.writeText "${name}-host.nft" ''
      table inet tunnel-${name}
      delete table inet tunnel-${name}
      table inet tunnel-${name} {
        chain prerouting {
          type nat hook prerouting priority dstnat; policy accept;
          ${lines (
            concatMap (
              mapping:
              map (
                protocol:
                ''
                  meta nfproto ipv4 fib daddr type local ${protocol} dport ${toString mapping.from} dnat ip to ${namespace.namespaceAddress}:${toString mapping.to}
                ''
                + optionalString ipv6 ''
                  meta nfproto ipv6 fib daddr type local ${protocol} dport ${toString mapping.from} dnat ip6 to [${namespace.namespaceAddressIPv6}]:${toString mapping.to}
                ''
              ) (protocols mapping.protocol)
            ) namespace.portMappings
          )}
        }
        chain postrouting {
          type nat hook postrouting priority srcnat; policy accept;
          ${optionalString namespace.hostAccess.enable ''oifname "${name}-br" meta mark ${toString namespace.hostAccess.mark} masquerade''}
          ${lines (
            map (
              network:
              if isV4 network then
                "ip saddr ${namespace.namespaceAddress} ip daddr ${network} masquerade"
              else
                "ip6 saddr ${namespace.namespaceAddressIPv6} ip6 daddr ${network} masquerade"
            ) namespace.allowedEgress
          )}
        }
      }
    '';

  path = lib.makeBinPath [
    pkgs.coreutils
    pkgs.iproute2
    pkgs.nftables
    pkgs.procps
    pkgs.wireguard-tools
  ];

  down =
    name: namespace:
    pkgs.writeShellScript "${name}-down" ''
      export PATH=${path}
      exec 2>/dev/null
      ${lines (map (interface: "ip -n ${name} link del ${interface}") namespace.interfaces)}
      ip netns del ${name}
      ${lines (map (interface: "ip link del ${interface}") namespace.interfaces)}
      ip link del ${name}-br
      ip link del veth-${name}-br
      rm -rf /etc/netns/${name}
      nft delete table inet tunnel-${name}
      ${optionalString namespace.hostAccess.enable ''
        ip rule del fwmark ${toString namespace.hostAccess.mark} lookup ${toString namespace.hostAccess.table}
        ip route flush table ${toString namespace.hostAccess.table}
      ''}
      exit 0
    '';

  up =
    name: namespace:
    let
      inside = "ip -n ${name}";
      routed = filter (network: ipv6 || isV4 network) (
        unique (namespace.accessibleFrom ++ namespace.allowedEgress)
      );
    in
    pkgs.writeShellScript "${name}-up" ''
      set -euo pipefail
      export PATH=${path}
      shopt -s nocasematch extglob

      trim() {
        local text=$1
        text=''${text##+([[:space:]])}
        printf '%s' "''${text%%+([[:space:]])}"
      }

      config=""
      endpoints=(${escapeShellArgs namespace.endpoints})
      addresses=()
      servers=()
      search=()
      mtu=""
      section=""
      while IFS= read -r line || [[ -n $line ]]; do
        stripped=''${line%%#*}
        key=$(trim "''${stripped%%=*}")
        value=$(trim "''${stripped#*=}")
        if [[ $key == \[*\] ]]; then
          section=$key
        fi
        if [[ $section == "[Interface]" ]]; then
          case $key in
            Address)
              read -ra parts <<< "''${value//,/ }"
              addresses+=("''${parts[@]}")
              continue
              ;;
            MTU)
              mtu=$value
              continue
              ;;
            DNS)
              read -ra parts <<< "''${value//,/ }"
              for part in "''${parts[@]}"; do
                if [[ $part =~ ^[0-9.]+$ || $part == *:* ]]; then
                  servers+=("$part")
                else
                  search+=("$part")
                fi
              done
              continue
              ;;
            Table | PreUp | PreDown | PostUp | PostDown | SaveConfig)
              continue
              ;;
          esac
        fi
        if [[ $key == Endpoint && ''${#endpoints[@]} -gt 0 ]]; then
          continue
        fi
        config+=$line$'\n'
      done < ${namespace.wireguardConfigFile}

      if [[ ''${#servers[@]} -eq 0 ]]; then
        echo "${name}: the WireGuard config sets no DNS server" >&2
        exit 1
      fi

      ip netns add ${name}
      ${inside} link set lo up
      ip netns exec ${name} nft -f ${insideRules name namespace}

      mkdir -p /etc/netns/${name}
      {
        printf 'nameserver %s\n' "''${servers[@]}"
        if [[ ''${#search[@]} -gt 0 ]]; then
          printf 'search %s\n' "''${search[*]}"
        fi
      } > /etc/netns/${name}/resolv.conf
      echo "hosts: files dns" > /etc/netns/${name}/nsswitch.conf
      for server in "''${servers[@]}"; do
        if [[ $server == *:* ]]; then
          ip netns exec ${name} nft add element inet confine dns6 "{ $server }"
        else
          ip netns exec ${name} nft add element inet confine dns4 "{ $server }"
        fi
      done

      interfaces=(${escapeShellArgs namespace.interfaces})
      for number in "''${!interfaces[@]}"; do
        interface=''${interfaces[$number]}
        tunnel=$config
        if [[ ''${#endpoints[@]} -gt 0 ]]; then
          tunnel+="Endpoint = ''${endpoints[$number]}"$'\n'
        fi
        ip link add "$interface" type wireguard
        wg setconf "$interface" <(printf '%s' "$tunnel")
        ${optionalString (
          namespace.transportMark != null
        ) ''wg set "$interface" fwmark ${toString namespace.transportMark}''}
        ip link set "$interface" netns ${name}
        for address in "''${addresses[@]}"; do
          ${inside} address add "$address" dev "$interface"
        done
        ${inside} link set "$interface" mtu ${
          if namespace.mtu != null then toString namespace.mtu else ''"''${mtu:-1420}"''
        } up
      done
      ${optionalString (length namespace.interfaces > 1) ''
        ip netns exec ${name} sysctl -wq net.ipv4.tcp_fwmark_accept=1 net.ipv4.fwmark_reflect=1 ${optionalString ipv6 "net.ipv6.fwmark_reflect=1"}
      ''}

      ip link add ${name}-br type bridge
      ip address add ${namespace.bridgeAddress}/24 dev ${name}-br
      ${optionalString ipv6 "ip address add ${namespace.bridgeAddressIPv6}/64 dev ${name}-br"}
      ip link set ${name}-br up
      ip link add veth-${name}-br type veth peer name veth-${name} netns ${name}
      ip link set veth-${name}-br master ${name}-br up
      ${inside} address add ${namespace.namespaceAddress}/24 dev veth-${name}
      ${optionalString ipv6 "${inside} address add ${namespace.namespaceAddressIPv6}/64 dev veth-${name}"}
      ${inside} link set veth-${name} up

      for family in -4 ${optionalString ipv6 "-6"}; do
        nexthop=()
        if [[ $family == -6 ]]; then
          # one neighbour entry for the tunnel instead of one per destination
          nexthop=(via fe80::1 onlink)
        fi
        hops=()
        for number in "''${!interfaces[@]}"; do
          interface=''${interfaces[$number]}
          ${inside} $family route add default dev "$interface" "''${nexthop[@]}" table $((51820 + number))
          hops+=(nexthop "''${nexthop[@]::2}" dev "$interface" weight 1 "''${nexthop[@]:2}")
          ${optionalString (length namespace.interfaces > 1) ''
            ${inside} $family rule add fwmark $((number + 1)) lookup $((51820 + number)) priority 90
          ''}
        done
        if [[ ''${#interfaces[@]} -eq 1 ]]; then
          ${inside} $family route add default dev ${name}0 "''${nexthop[@]}"
        else
          ${inside} $family route add default "''${hops[@]}"
        fi
        ${inside} $family rule add ipproto udp dport 53 lookup 51820 priority 100
        ${inside} $family rule add ipproto tcp dport 53 lookup 51820 priority 100
      done
      ${lines (
        map (
          network:
          "${inside} route add ${network} via ${
            if isV4 network then namespace.bridgeAddress else namespace.bridgeAddressIPv6
          }"
        ) routed
      )}

      nft -f ${hostRules name namespace}
      ${optionalString namespace.hostAccess.enable ''
        ip netns exec ${name} sysctl -wq net.ipv4.ip_forward=1
        ip route replace default via ${namespace.namespaceAddress} dev ${name}-br table ${toString namespace.hostAccess.table}
        ip rule add fwmark ${toString namespace.hostAccess.mark} lookup ${toString namespace.hostAccess.table} priority ${toString namespace.hostAccess.table}
      ''}
    '';
in
{
  options = {
    vpnNamespaces = mkOption {
      type = types.attrsOf (types.submodule namespaceModule);
      default = { };
      description = "Network namespaces whose only way out is a WireGuard tunnel.";
    };

    systemd.services = mkOption {
      type = types.attrsOf (
        types.submodule (
          { config, ... }:
          let
            namespace = config.vpnConfinement.vpnNamespace;
          in
          {
            options.vpnConfinement = {
              enable = mkEnableOption "confining the service to a tunnel namespace";
              vpnNamespace = mkOption {
                type = types.str;
                description = "Tunnel namespace the service runs in.";
              };
            };

            config = mkIf config.vpnConfinement.enable {
              bindsTo = [ "${namespace}.service" ];
              after = [ "${namespace}.service" ];
              serviceConfig = {
                NetworkNamespacePath = "/run/netns/${namespace}";
                InaccessiblePaths = [
                  "/run/nscd"
                  "/var/run/nscd"
                  "/run/resolvconf"
                  "/var/run/resolvconf"
                ];
                TemporaryFileSystem = [
                  "/run/nscd:ro"
                  "/var/run/nscd:ro"
                  "/run/resolvconf:ro"
                  "/var/run/resolvconf:ro"
                ];
                BindReadOnlyPaths = [
                  "/etc/netns/${namespace}/resolv.conf:/etc/resolv.conf:norbind"
                  "/etc/netns/${namespace}/nsswitch.conf:/etc/nsswitch.conf:norbind"
                ];
                NoNewPrivileges = true;
                RestrictAddressFamilies = [
                  "AF_INET"
                  "AF_INET6"
                  "AF_UNIX"
                ];
              };
            };
          }
        )
      );
    };
  };

  config = mkIf (namespaces != { }) {
    assertions = lib.mapAttrsToList (name: _: {
      assertion = builtins.stringLength name < 8;
      message = "vpnNamespaces.${name}: the name must be shorter than 8 characters to fit interface names.";
    }) namespaces;

    boot.kernel.sysctl = {
      "net.ipv4.ip_forward" = 1;
      "net.ipv6.conf.all.forwarding" = mkIf ipv6 1;
    };

    systemd.tmpfiles.rules = [ "d /run/resolvconf 0755 root root" ];

    systemd.services = mapAttrs (name: namespace: {
      description = "${name} network interface";
      after = [ "network-online.target" ];
      wants = [ "network-online.target" ];
      serviceConfig = {
        Type = "oneshot";
        RemainAfterExit = true;
        ExecStartPre = [ (down name namespace) ];
        ExecStart = up name namespace;
        ExecStopPost = down name namespace;
      };
    }) namespaces;
  };
}
