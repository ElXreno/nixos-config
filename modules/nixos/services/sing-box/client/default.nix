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
    sops = {
      secrets = {
        "vless/${namespace}-uuid" = {
          owner = "sing-box";
          group = "sing-box";
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
          servers = [
            {
              tag = "local-dns";
              type = "local";
            }
          ];
        };

        inbounds = [
          {
            type = "tun";
            domain_strategy = "prefer_ipv4";
            address = [ "172.16.0.1/30" ];
            stack = "gvisor";
            auto_route = true;
            strict_route = true;

            route_address = [
              "0.0.0.0/0"
              "::/0"
            ];

            route_exclude_address = [
              "10.0.0.0/8"
              "172.16.0.0/12"
              "100.64.0.0/10"
              "200::/7"
              "fd00::/8"
              "127.0.0.69/32"

              # cloudflare & google dns
              "1.1.1.1/32"
              "8.8.8.8/32"
            ];
          }
        ];

        outbounds = [
          {
            tag = "vless-out";
            type = "vless";

            server = "datalake.elxreno.com";
            server_port = 443;

            domain_resolver = {
              server = "local-dns";
              domain_strategy = "prefer_ipv4";
            };

            uuid._secret = config.sops.secrets."vless/${namespace}-uuid".path;

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
          {
            type = "direct";
            tag = "direct";
          }
        ];

        route = {
          rules = [
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
            {
              action = "sniff";
            }
            {
              protocol = "dns";
              action = "hijack-dns";
            }
            {
              protocol = "bittorrent";
              action = "reject";
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

          final = "vless-out";
          auto_detect_interface = true;
        };

        experimental.cache_file.enabled = true;
      };
    };

    systemd.services.sing-box.wantedBy = mkIf (!cfg.autostart) (lib.mkForce [ ]);
  };
}
