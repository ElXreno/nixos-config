{
  config,
  namespace,
  lib,
  ...
}:

let
  inherit (lib) mkIf mkEnableOption;
  cfg = config.${namespace}.services.sing-box.client;
in
{
  options.${namespace}.services.sing-box.client = {
    enable = mkEnableOption "Whether or not to manage sing-box client.";
    autostart = mkEnableOption "Whether to execute sing-box client at system boot.";
  };

  config = mkIf cfg.enable {
    clan.core.vars.generators.sing-box-uuid = {
      files.uuid.secret = true;
      script = ''
        cat /proc/sys/kernel/random/uuid > "$out/uuid"
      '';
    };

    clan.core.vars.generators.sing-box-socks = {
      prompts = {
        address = {
          description = "IP Address of a SOCKS5 proxy";
          type = "hidden";
          persist = true;
        };
        username = {
          description = "Username of a SOCKS5 proxy";
          type = "hidden";
          persist = true;
        };
        password = {
          description = "Password of a SOCKS5 proxy";
          type = "hidden";
          persist = true;
        };
      };
    };

    services.sing-box = {
      enable = true;
      settings = {
        log = {
          level = "debug";
        };

        dns = {
          strategy = "ipv4_only";
          servers = [
            {
              type = "https";
              tag = "dns-proxy";
              server = "1.1.1.1";
              detour = "proxy-seller";
            }
            {
              type = "udp";
              tag = "dns-direct";
              server = "127.0.0.1";
            }
          ];
          rules = [
            {
              domain_suffix = [
                "anthropic.com"
                "clau.de"
                "claude.ai"
                "claude.com"
                "claudemcpclient.com"
                "claudeusercontent.com"
                "sentry.io"
                "servd-anthropic-website.b-cdn.net"
                "statsig.com"
                "stripe.com"
                "usefathom.com"
              ];
              action = "route";
              server = "dns-proxy";
            }
          ];
          final = "dns-direct";
        };

        inbounds = [
          {
            type = "tun";

            address = [
              "172.18.0.1/30"
            ];

            mtu = 65535;
            auto_route = true;
            auto_redirect = true;
            strict_route = true;

            stack = "system";

            route_address = [
              "0.0.0.0/0"
              "::/0"
            ];

            exclude_interface = [ "tailscale0" ];

            route_exclude_address = [
              "10.0.0.0/8"
              "172.16.0.0/12"
              "100.64.0.0/10"

              # DNS
              "1.1.1.1/32"
              "1.0.0.1/32"
              "8.8.8.8/32"
              "8.8.4.4/32"

              # IPv6 loopback
              "::1/128"

              # Yggdrasil
              "200::/7"

              # link-local multicast
              "ff02::/16"
              # link-local unicast
              "fe80::/10"
            ];
          }
        ];

        outbounds = [
          {
            tag = "vless-out";
            type = "vless";

            server = "datalake.elxreno.com";
            server_port = 443;

            uuid._secret = config.clan.core.vars.generators."sing-box-uuid".files.uuid.path;

            tls = {
              enabled = true;
              server_name = "datalake.elxreno.com";
              utls = {
                enabled = true;
                fingerprint = "chrome";
              };
            };

            transport = {
              type = "ws";
              path = "/api/v1/stream";
              max_early_data = 2048;
              early_data_header_name = "Sec-WebSocket-Protocol";
              headers.Host = [ "datalake.elxreno.com" ];
            };
          }
          (with config.clan.core.vars.generators.sing-box-socks.files; {
            type = "socks";
            tag = "proxy-seller";
            server._secret = address.path;
            server_port = 50101;
            username._secret = username.path;
            password._secret = password.path;
            version = "5";
          })
          {
            type = "direct";
            tag = "direct";
          }
        ];

        route = {
          default_domain_resolver = {
            server = "dns-direct";
            domain_strategy = "prefer_ipv4";
          };

          rules = [
            {
              process_name = [
                "dnscrypt-proxy"
                "syncthing"
                "qbittorrent"
                "qbittorrent-nox"
                ".qbittorrent-nox-wrapped"
              ];
              outbound = "direct";
            }
            {
              action = "sniff";
            }
            {
              protocol = "dns";
              action = "hijack-dns";
            }
            {
              domain_suffix = [
                "anthropic.com"
                "clau.de"
                "claude.ai"
                "claude.com"
                "claudemcpclient.com"
                "claudeusercontent.com"
                "sentry.io"
                "servd-anthropic-website.b-cdn.net"
                "statsig.com"
                "stripe.com"
                "usefathom.com"

                # Arr stack metadata APIs (Cloudflare-blocked)
                "api.radarr.video"
                "api.sonarr.tv"
              ];
              outbound = "proxy-seller";
            }
            {
              domain_suffix = [
                ".ru"
                ".by"
              ];
              rule_set = [
                "geoip-ru"
                "geoip-by"
              ];
              outbound = "direct";
            }
          ];

          rule_set = [
            {
              tag = "geoip-ru";
              type = "remote";
              format = "binary";
              url = "https://raw.githubusercontent.com/SagerNet/sing-geoip/rule-set/geoip-ru.srs";
              download_detour = "direct";
            }
            {
              tag = "geoip-by";
              type = "remote";
              format = "binary";
              url = "https://raw.githubusercontent.com/SagerNet/sing-geoip/rule-set/geoip-by.srs";
              download_detour = "direct";
            }
          ];

          final = "direct";
          auto_detect_interface = true;
        };

        experimental = {
          cache_file.enabled = true;
          clash_api = {
            external_controller = "127.0.0.1:9090";
          };
          debug = {
            listen = "127.0.0.1:9091";
          };
        };
      };
    };

    # CAP_NET_ADMIN + CAP_NET_RAW: tun device, routing
    # CAP_BPF + CAP_PERFMON:       load eBPF fentry/fexit tracing programs
    # CAP_SYS_PTRACE:              readlink /proc/<pid>/exe across UIDs
    systemd.services.sing-box.serviceConfig = {
      CapabilityBoundingSet = [
        "CAP_NET_ADMIN"
        "CAP_NET_RAW"
        "CAP_BPF"
        "CAP_PERFMON"
        "CAP_SYS_PTRACE"
      ];
      AmbientCapabilities = [
        "CAP_NET_ADMIN"
        "CAP_NET_RAW"
        "CAP_BPF"
        "CAP_PERFMON"
        "CAP_SYS_PTRACE"
      ];
    };

    systemd.services.sing-box.wantedBy = mkIf (!cfg.autostart) (lib.mkForce [ ]);
  };
}
