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
        iptables -w -A nixos-fw -p tcp --syn ${ignoredPortsArg} -m recent --name honeypot_candidates --update --seconds ${toString cfg.hitSeconds} --hitcount ${toString cfg.hitCount} -j SET --add-set blocked src
        iptables -w -A nixos-fw -p tcp --syn ${ignoredPortsArg} -m recent --name honeypot_candidates --set -j DROP
        iptables -w -A nixos-fw -p udp ${ignoredPortsArg} -m recent --name honeypot_candidates --update --seconds ${toString cfg.hitSeconds} --hitcount ${toString cfg.hitCount} -j SET --add-set blocked src
        iptables -w -A nixos-fw -p udp ${ignoredPortsArg} -m recent --name honeypot_candidates --set -j DROP

        ip6tables -w -I INPUT 1 -m set --match-set blocked6 src -j DROP
        ip6tables -w -A nixos-fw -p tcp --syn ${ignoredPortsArg} -m recent --name honeypot_candidates --update --seconds ${toString cfg.hitSeconds} --hitcount ${toString cfg.hitCount} -j SET --add-set blocked6 src
        ip6tables -w -A nixos-fw -p tcp --syn ${ignoredPortsArg} -m recent --name honeypot_candidates --set -j DROP
        ip6tables -w -A nixos-fw -p udp ${ignoredPortsArg} -m recent --name honeypot_candidates --update --seconds ${toString cfg.hitSeconds} --hitcount ${toString cfg.hitCount} -j SET --add-set blocked6 src
        ip6tables -w -A nixos-fw -p udp ${ignoredPortsArg} -m recent --name honeypot_candidates --set -j DROP
      '';

      extraStopCommands = ''
        iptables -w -D nixos-fw -p tcp --syn ${ignoredPortsArg} -m recent --name honeypot_candidates --set -j DROP 2>/dev/null || true
        iptables -w -D nixos-fw -p tcp --syn ${ignoredPortsArg} -m recent --name honeypot_candidates --update --seconds ${toString cfg.hitSeconds} --hitcount ${toString cfg.hitCount} -j SET --add-set blocked src 2>/dev/null || true
        iptables -w -D nixos-fw -p udp ${ignoredPortsArg} -m recent --name honeypot_candidates --set -j DROP 2>/dev/null || true
        iptables -w -D nixos-fw -p udp ${ignoredPortsArg} -m recent --name honeypot_candidates --update --seconds ${toString cfg.hitSeconds} --hitcount ${toString cfg.hitCount} -j SET --add-set blocked src 2>/dev/null || true
        iptables -w -D INPUT -m set --match-set blocked src -j DROP 2>/dev/null || true

        ip6tables -w -D nixos-fw -p tcp --syn ${ignoredPortsArg} -m recent --name honeypot_candidates --set -j DROP 2>/dev/null || true
        ip6tables -w -D nixos-fw -p tcp --syn ${ignoredPortsArg} -m recent --name honeypot_candidates --update --seconds ${toString cfg.hitSeconds} --hitcount ${toString cfg.hitCount} -j SET --add-set blocked6 src 2>/dev/null || true
        ip6tables -w -D nixos-fw -p udp ${ignoredPortsArg} -m recent --name honeypot_candidates --set -j DROP 2>/dev/null || true
        ip6tables -w -D nixos-fw -p udp ${ignoredPortsArg} -m recent --name honeypot_candidates --update --seconds ${toString cfg.hitSeconds} --hitcount ${toString cfg.hitCount} -j SET --add-set blocked6 src 2>/dev/null || true
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
