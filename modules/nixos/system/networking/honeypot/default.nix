{
  config,
  namespace,
  lib,
  pkgs,
  ...
}:
let
  inherit (lib)
    mkIf
    mkEnableOption
    mkOption
    types
    concatStringsSep
    ;
  cfg = config.${namespace}.system.networking.honeypot;

  ipsetPath = "${cfg.ipsetDirectory}/ipset.conf";
  ignoredPortsArg =
    if cfg.ignoredPorts == [ ] then
      ""
    else
      "-m multiport ! --dports ${concatStringsSep "," (map toString cfg.ignoredPorts)}";

  whitelistRules4 = concatStringsSep "\n" (
    map (ip: "iptables -w -A nixos-fw -s ${ip} -j RETURN") cfg.whitelistedIPs.ipv4
  );
  whitelistRules6 = concatStringsSep "\n" (
    map (ip: "ip6tables -w -A nixos-fw -s ${ip} -j RETURN") cfg.whitelistedIPs.ipv6
  );
  whitelistStopRules4 = concatStringsSep "\n" (
    map (ip: "iptables -w -D nixos-fw -s ${ip} -j RETURN 2>/dev/null || true") cfg.whitelistedIPs.ipv4
  );
  whitelistStopRules6 = concatStringsSep "\n" (
    map (ip: "ip6tables -w -D nixos-fw -s ${ip} -j RETURN 2>/dev/null || true") cfg.whitelistedIPs.ipv6
  );
in
{
  options.${namespace}.system.networking.honeypot = {
    enable = mkEnableOption "Whether to enable simple honeypot.";
    blockDurationSeconds = mkOption {
      type = types.int;
      default = 60 * 60 * 24 * 20; # 20 days
      description = "Block duration (in seconds).";
    };
    hitCount = mkOption {
      type = types.int;
      default = 3;
      description = "Number of packets to closed ports before banning the IP.";
    };
    hitSeconds = mkOption {
      type = types.int;
      default = 60;
      description = ''
        The time window (in seconds) during which `hitCount` must be reached
        to trigger a ban. Each new packet within this window resets the timer.
      '';
    };
    ignoredPorts = mkOption {
      type = types.listOf types.port;
      default = [ ];
      description = ''
        Destination ports that should be ignored by honeypot accounting.
        Useful for excluding noisy probes from ban logic.
      '';
    };
    whitelistedIPs = {
      ipv4 = mkOption {
        type = types.listOf types.str;
        default = [ ];
        description = "IPv4 addresses/subnets that should never be banned by the honeypot.";
      };
      ipv6 = mkOption {
        type = types.listOf types.str;
        default = [ ];
        description = "IPv6 addresses/subnets that should never be banned by the honeypot.";
      };
    };
    ipsetDirectory = mkOption {
      type = types.str;
      default = "/var/lib/honeypot";
      description = "Where to store banned ips.";
    };
  };

  config = mkIf cfg.enable {
    systemd.tmpfiles.rules = [
      "d ${cfg.ipsetDirectory} 0700 root root -"
    ];

    networking.firewall = {
      extraPackages = [ pkgs.ipset ];

      extraCommands = ''
        if test -f ${ipsetPath}; then
            ipset restore -! < ${ipsetPath}
        else
            ipset -exist create blocked hash:ip hashsize 4096 maxelem 300000 ${
              if cfg.blockDurationSeconds > 0 then "timeout ${toString cfg.blockDurationSeconds}" else ""
            }
            ipset -exist create blocked6 hash:ip hashsize 4096 maxelem 300000 family inet6 ${
              if cfg.blockDurationSeconds > 0 then "timeout ${toString cfg.blockDurationSeconds}" else ""
            }
        fi

        iptables -w -I INPUT 1 -m set --match-set blocked src -j DROP
        ip6tables -w -I INPUT 1 -m set --match-set blocked6 src -j DROP

        # Honeypot chain: skip private sources, then track and ban scanners
        iptables -w -N honeypot 2>/dev/null || iptables -w -F honeypot
        iptables -w -A honeypot -s 10.0.0.0/8 -j RETURN
        iptables -w -A honeypot -s 172.16.0.0/12 -j RETURN
        iptables -w -A honeypot -s 192.168.0.0/16 -j RETURN
        iptables -w -A honeypot -s 127.0.0.0/8 -j RETURN
        iptables -w -A honeypot -p tcp --syn ${ignoredPortsArg} -m recent --name honeypot_tcp --update --seconds ${toString cfg.hitSeconds} --hitcount ${toString cfg.hitCount} -j SET --add-set blocked src
        iptables -w -A honeypot -p tcp --syn ${ignoredPortsArg} -m recent --name honeypot_tcp --set -j DROP
        iptables -w -A honeypot -p udp ${ignoredPortsArg} -m recent --name honeypot_udp --update --seconds ${toString cfg.hitSeconds} --hitcount ${toString cfg.hitCount} -j SET --add-set blocked src
        iptables -w -A honeypot -p udp ${ignoredPortsArg} -m recent --name honeypot_udp --set -j DROP

        ip6tables -w -N honeypot 2>/dev/null || ip6tables -w -F honeypot
        ip6tables -w -A honeypot -s fe80::/10 -j RETURN
        ip6tables -w -A honeypot -s fc00::/7 -j RETURN
        ip6tables -w -A honeypot -s ::1 -j RETURN
        ip6tables -w -A honeypot -p tcp --syn ${ignoredPortsArg} -m recent --name honeypot_tcp --update --seconds ${toString cfg.hitSeconds} --hitcount ${toString cfg.hitCount} -j SET --add-set blocked6 src
        ip6tables -w -A honeypot -p tcp --syn ${ignoredPortsArg} -m recent --name honeypot_tcp --set -j DROP
        ip6tables -w -A honeypot -p udp ${ignoredPortsArg} -m recent --name honeypot_udp --update --seconds ${toString cfg.hitSeconds} --hitcount ${toString cfg.hitCount} -j SET --add-set blocked6 src
        ip6tables -w -A honeypot -p udp ${ignoredPortsArg} -m recent --name honeypot_udp --set -j DROP

        # Jump from nixos-fw to honeypot chain
        ${whitelistRules4}
        iptables -w -A nixos-fw -j honeypot
        ${whitelistRules6}
        ip6tables -w -A nixos-fw -j honeypot
      '';

      extraStopCommands = ''
        iptables -w -D nixos-fw -j honeypot 2>/dev/null || true
        ${whitelistStopRules4}
        iptables -w -F honeypot 2>/dev/null || true
        iptables -w -X honeypot 2>/dev/null || true
        iptables -w -D INPUT -m set --match-set blocked src -j DROP 2>/dev/null || true

        ip6tables -w -D nixos-fw -j honeypot 2>/dev/null || true
        ${whitelistStopRules6}
        ip6tables -w -F honeypot 2>/dev/null || true
        ip6tables -w -X honeypot 2>/dev/null || true
        ip6tables -w -D INPUT -m set --match-set blocked6 src -j DROP 2>/dev/null || true

        ipset -exist create blocked hash:ip hashsize 4096 maxelem 300000 ${
          if cfg.blockDurationSeconds > 0 then "timeout ${toString cfg.blockDurationSeconds}" else ""
        }
        ipset -exist create blocked6 hash:ip hashsize 4096 maxelem 300000 family inet6 ${
          if cfg.blockDurationSeconds > 0 then "timeout ${toString cfg.blockDurationSeconds}" else ""
        }

        ipset save > ${ipsetPath}

        ipset destroy blocked 2>/dev/null || true
        ipset destroy blocked6 2>/dev/null || true
      '';
    };
  };
}
