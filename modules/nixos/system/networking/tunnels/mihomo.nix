{
  config,
  namespace,
  lib,
  pkgs,
  ...
}:

let
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
    genAttrs
    groupBy
    hasPrefix
    imap0
    length
    listToAttrs
    mapAttrsToList
    mkEnableOption
    mkForce
    mkIf
    mkMerge
    mkOption
    nameValuePair
    optional
    optionalString
    removePrefix
    replaceStrings
    sort
    splitString
    types
    unique
    ;
  cfg = config.${namespace}.system.networking;
  ports = config.${namespace}.ports;

  domains = filterAttrs (_: entries: entries != [ ]) cfg.routes.domains;
  isNode = name: cfg.tunnels.${name}.type == "hysteria2";
  nodes = filter isNode (unique (attrNames domains ++ cfg.routes.cgroups));
  marked = filter (name: !isNode name) (attrNames domains);
  viaNodes = filter isNode cfg.routes.cgroups;
  netns = attrNames (
    filterAttrs (_: tunnel: tunnel.enable && tunnel.type == "wireguard") cfg.tunnels
  );

  isSet = hasPrefix "geosite:";
  usesSets = any isSet (concatLists (attrValues domains));
  depth = entry: if isSet entry then 0 else length (splitString "." entry);
  rule =
    tunnel: entry:
    if isSet entry then
      "GEOSITE,${removePrefix "geosite:" entry},${tunnel}"
    else
      "DOMAIN-SUFFIX,${entry},${tunnel}";

  rules = map (entry: rule entry.tunnel entry.name) (
    sort (a: b: a.depth > b.depth) (
      concatLists (
        mapAttrsToList (
          tunnel:
          map (name: {
            inherit tunnel name;
            depth = depth name;
          })
        ) domains
      )
    )
  );

  duplicates = attrNames (
    filterAttrs (_: tunnels: length tunnels > 1) (
      groupBy (pair: pair.entry) (
        concatLists (
          mapAttrsToList (
            tunnel:
            map (entry: {
              inherit tunnel entry;
            })
          ) domains
        )
      )
    )
  );

  udpMark = 16896;
  udpTable = 4200;
  viaSet = name: "via_${replaceStrings [ "-" ] [ "_" ] name}";
  viaMark = name: udpMark + 1 + lib.lists.findFirstIndex (other: other == name) null viaNodes;
  tailscaleMark = "meta mark & 0xff0000 == 0x80000";
  tunnelMarks = [
    cfg.tunnelTransportMark
  ]
  ++ map (tunnel: tunnel.hostAccess.mark) (
    filter (tunnel: tunnel.enable && tunnel.type == "wireguard" && tunnel.hostAccess.enable) (
      attrValues cfg.tunnels
    )
  );

  geosite = "${pkgs.v2ray-domain-list-community}/share/v2ray/geosite.dat";
  stateDir = "/var/lib/private/mihomo";
  configFile = "/run/mihomo-config/config.yaml";
  apiSecret = config.clan.core.vars.generators.mihomo-api.files.secret.path;

  settings = {
    mode = "rule";
    log-level = "info";
    allow-lan = false;
    ipv6 = false;
    tcp-concurrent = true;
    find-process-mode = "off";
    geodata-mode = true;
    geo-auto-update = false;
    redir-port = ports.router-redir;
    tproxy-port = ports.router-tproxy;
    external-controller-unix = "/run/mihomo/api.sock";
    external-controller = "127.0.0.1:${toString ports.router-api}";
    secret = "@api-secret@";
    profile = {
      store-selected = false;
      store-fake-ip = false;
    };
    dns = {
      enable = true;
      listen = "127.0.0.1:${toString ports.router-dns}";
      ipv6 = false;
      enhanced-mode = "redir-host";
      nameserver = [ cfg.routes.dnsUpstream ];
    };
    sniffer = {
      enable = true;
      override-destination = true;
      parse-pure-ip = true;
      sniff = {
        TLS.ports = [
          443
          8443
        ];
        HTTP.ports = [
          80
          "8080-8880"
        ];
        QUIC = {
          ports = [
            443
            8443
          ];
          override-destination = false;
        };
      };
    };
    listeners = concatMap (name: [
      {
        name = "via-${name}";
        type = "redir";
        listen = "127.0.0.1";
        port = ports."router-via-${name}";
        proxy = name;
      }
      {
        name = "via-${name}-udp";
        type = "tproxy";
        listen = "127.0.0.1";
        port = ports."router-via-${name}-udp";
        proxy = name;
      }
    ]) viaNodes;
    proxies =
      map (
        name:
        let
          tunnel = cfg.tunnels.${name};
        in
        {
          inherit name;
          type = "hysteria2";
          inherit (tunnel) server port;
          password = "@${name}@";
          sni = tunnel.server;
        }
      ) nodes
      ++ map (name: {
        inherit name;
        type = "direct";
        routing-mark = cfg.tunnels.${name}.hostAccess.mark;
      }) marked;
    rules = rules ++ [ "MATCH,DIRECT" ];
  };

  unchecked = pkgs.writeText "mihomo.yaml.in" (builtins.toJSON settings);

  template =
    pkgs.runCommand "mihomo.yaml.in"
      {
        nativeBuildInputs = [ config.services.mihomo.package ];
      }
      ''
        mkdir home
        ${optionalString usesSets "ln -s ${geosite} home/GeoSite.dat"}
        mihomo -t -d home -f ${unchecked}
        cp ${unchecked} $out
      '';

  render = pkgs.writeShellScript "mihomo-config" ''
    umask 077
    text=$(<${template})
    ${concatMapStringsSep "\n" (name: ''
      secret=$(<${cfg.tunnels.${name}.passwordFile})
      text=''${text//@${name}@/$secret}
    '') nodes}
    secret=$(<${apiSecret})
    text=''${text//@api-secret@/$secret}
    printf '%s\n' "$text" > "$RUNTIME_DIRECTORY/config.yaml"
  '';

  ip = "${pkgs.iproute2}/bin/ip";
  nft = "${pkgs.nftables}/bin/nft";
  flushDns = "+-${config.systemd.package}/bin/resolvectl flush-caches";

  ready = pkgs.writeShellScript "mihomo-ready" ''
    for _ in $(${pkgs.coreutils}/bin/seq 50); do
      if (exec 3<>/dev/tcp/127.0.0.1/${toString ports.router-dns}) 2>/dev/null; then
        exit 0
      fi
      ${pkgs.coreutils}/bin/sleep 0.1
    done
    exit 1
  '';

  capture = [
    "tcp-out"
    "udp-out"
    "ipv6-out"
  ];

  table = pkgs.writeText "router.nft" ''
    add table inet router
    add set inet router bypass { type cgroupsv2; }
    ${concatMapStringsSep "\n" (
      name: "add set inet router ${viaSet name} { type cgroupsv2; }"
    ) viaNodes}
    add chain inet router exempt
    add chain inet router tcp-out { type nat hook output priority dstnat; policy accept; }
    add chain inet router udp-out { type route hook output priority mangle + 5; policy accept; }
    add chain inet router ipv6-out { type filter hook output priority filter; policy accept; }
    add chain inet router divert { type filter hook prerouting priority mangle; policy accept; }
    flush chain inet router exempt
    flush chain inet router divert
    ${concatMapStringsSep "\n" (chain: "flush chain inet router ${chain}") capture}

    add rule inet router exempt ct direction reply accept
    add rule inet router exempt fib daddr type { local, broadcast, multicast, anycast } accept
    add rule inet router exempt ip daddr { 10.0.0.0/8, 172.16.0.0/12, 192.168.0.0/16, 100.64.0.0/10, 169.254.0.0/16 } accept
    add rule inet router exempt ip6 daddr { fe80::/10, fc00::/7 } accept
    add rule inet router exempt socket cgroupv2 level 2 @bypass accept
    add rule inet router exempt ${tailscaleMark} accept
    add rule inet router exempt meta mark { ${concatMapStringsSep ", " toString tunnelMarks} } accept

    add rule inet router tcp-out meta l4proto != tcp accept
    add rule inet router tcp-out jump exempt
    add rule inet router tcp-out meta nfproto != ipv4 accept
    ${concatMapStringsSep "\n" (
      name:
      "add rule inet router tcp-out socket cgroupv2 level 2 @${viaSet name} meta l4proto tcp redirect to :${
        toString ports."router-via-${name}"
      }"
    ) viaNodes}
    add rule inet router tcp-out meta l4proto tcp redirect to :${toString ports.router-redir}

    add rule inet router udp-out meta l4proto != udp accept
    add rule inet router udp-out jump exempt
    add rule inet router udp-out meta nfproto != ipv4 accept
    ${concatMapStringsSep "\n" (
      name:
      "add rule inet router udp-out socket cgroupv2 level 2 @${viaSet name} meta mark set ${toString (viaMark name)} accept"
    ) viaNodes}
    add rule inet router udp-out meta mark set ${toString udpMark}

    add rule inet router ipv6-out meta nfproto != ipv6 accept
    add rule inet router ipv6-out jump exempt
    add rule inet router ipv6-out meta l4proto { tcp, udp } reject

    ${concatMapStringsSep "\n" (
      name:
      "add rule inet router divert meta l4proto udp meta mark ${toString (viaMark name)} tproxy ip to 127.0.0.1:${
        toString ports."router-via-${name}-udp"
      } accept"
    ) viaNodes}
    add rule inet router divert meta l4proto udp meta mark ${toString udpMark} tproxy ip to 127.0.0.1:${toString ports.router-tproxy} accept
  '';

  route = "fwmark ${toString udpMark}/0xff00 lookup ${toString udpTable} priority ${toString udpTable}";

  setup = pkgs.writeShellScript "router-rules" ''
    set -e
    ${nft} -f ${table}
    ${ip} -4 rule del ${route} 2>/dev/null || true
    ${ip} -4 rule add ${route}
    ${ip} -4 route replace local default dev lo table ${toString udpTable}
  '';

  open = pkgs.writeText "router-open.nft" (
    concatMapStringsSep "\n" (chain: "flush chain inet router ${chain}") capture
  );
in
{
  options.${namespace}.system.networking.routes = {
    domains = mkOption {
      type = types.attrsOf (types.listOf types.str);
      default = { };
      example = {
        home = [
          "github.com"
          "geosite:microsoft"
        ];
      };
      description = "Domains routed through a tunnel for every program; a suffix or a geosite set.";
    };
    autostart = mkEnableOption "starting the domain router at boot" // {
      default = true;
    };
    dnsUpstream = mkOption {
      type = types.str;
      default = "127.0.0.1:53";
      description = "Resolver the domain router forwards queries to, as address:port.";
    };
    killSwitch = mkEnableOption "blocking non-local traffic while the domain router is down" // {
      default = true;
    };
  };

  config = mkIf (domains != { } || viaNodes != [ ]) {
    assertions = [
      {
        assertion = duplicates == [ ];
        message = "routes.domains lists a domain under several tunnels: ${concatStringsSep ", " duplicates}.";
      }
      {
        assertion = lib.all (
          name: cfg.tunnels.${name}.server != "" && cfg.tunnels.${name}.passwordFile != ""
        ) nodes;
        message = "A hysteria2 tunnel used by the domain router needs server and passwordFile.";
      }
    ];

    ${namespace}.ports = {
      router-dns = 5300;
      router-redir = 12345;
      router-tproxy = 12346;
      router-api = 12347;
    }
    // listToAttrs (
      concatLists (
        imap0 (position: name: [
          (nameValuePair "router-via-${name}" (12350 + 2 * position))
          (nameValuePair "router-via-${name}-udp" (12351 + 2 * position))
        ]) viaNodes
      )
    );

    clan.core.vars.generators.mihomo-api = {
      files.secret.owner = config.${namespace}.primaryUser;
      runtimeInputs = [ pkgs.openssl ];
      script = ''
        printf %s "$(openssl rand -hex 32)" > "$out/secret"
      '';
    };

    services.mihomo = {
      enable = true;
      inherit configFile;
      tunMode = true;
      webui = pkgs.metacubexd;
    };

    services.resolved.settings.Resolve.DNS = mkForce [ "127.0.0.1:${toString ports.router-dns}" ];

    systemd.network.config.networkConfig.ManageForeignRoutingPolicyRules = false;

    networking.firewall.extraReversePathFilterRules = ''
      iifname "lo" meta mark & 0xff00 == ${toString udpMark} accept
    '';

    systemd.slices = listToAttrs (
      map (
        name:
        nameValuePair "via-${name}" {
          requires = [ "router-rules.service" ];
          after = [ "router-rules.service" ];
          sliceConfig.NFTSet = [ "cgroup:inet:router:${viaSet name}" ];
        }
      ) viaNodes
    );

    systemd.services = mkMerge [
      (genAttrs netns (_: {
        after = [ "mihomo.service" ];
      }))
      {
        router-rules = {
          wantedBy = [ "multi-user.target" ];
          wants = [ "network-pre.target" ];
          before = [ "network-pre.target" ];
          serviceConfig = {
            Type = "oneshot";
            RemainAfterExit = true;
            ExecStart = setup;
          };
        };

        mihomo-config = {
          requiredBy = [ "mihomo.service" ];
          before = [ "mihomo.service" ];
          serviceConfig = {
            Type = "oneshot";
            RemainAfterExit = true;
            RuntimeDirectory = "mihomo-config";
            RuntimeDirectoryMode = "0700";
            ExecStart = render;
          };
        };

        mihomo = {
          requires = [ "router-rules.service" ];
          after = [
            "router-rules.service"
            "dnscrypt-proxy.service"
          ];
          wantedBy = mkIf (!cfg.routes.autostart) (mkForce [ ]);
          restartTriggers = [ template ];
          serviceConfig = {
            Restart = "on-failure";
            RestartSec = 2;
            RuntimeDirectory = "mihomo";
            NFTSet = [ "cgroup:inet:router:bypass" ];
            AmbientCapabilities = [ "CAP_NET_BIND_SERVICE" ];
            CapabilityBoundingSet = [ "CAP_NET_BIND_SERVICE" ];
            ExecStartPre = optional usesSets "${pkgs.coreutils}/bin/ln -sf ${geosite} ${stateDir}/GeoSite.dat";
            ExecStartPost = [
              ready
              flushDns
            ]
            ++ optional (!cfg.routes.killSwitch) "+${setup}";
            ExecStopPost = [ flushDns ] ++ optional (!cfg.routes.killSwitch) "+${nft} -f ${open}";
          };
        };

        dnscrypt-proxy = mkIf config.services.dnscrypt-proxy.enable {
          wants = [ "router-rules.service" ];
          after = [ "router-rules.service" ];
          serviceConfig.NFTSet = [ "cgroup:inet:router:bypass" ];
        };
      }
    ];
  };
}
