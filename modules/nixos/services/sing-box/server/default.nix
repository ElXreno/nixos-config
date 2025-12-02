{
  config,
  namespace,
  lib,
  ...
}:

let
  inherit (lib)
    mkIf
    mkEnableOption
    mkOption
    types
    listToAttrs
    nameValuePair
    ;
  cfg = config.${namespace}.services.sing-box.server;
in
{
  options.${namespace}.services.sing-box.server = {
    enable = mkEnableOption "Whether or not to manage sing-box server.";
    clients = mkOption {
      type = types.listOf types.str;
      default = [
        namespace
        "mobile"
        "noncere"
        "ierm"
        "ierm-extra"
        "ark"
      ];
    };
  };

  config = mkIf cfg.enable {
    sops = {
      secrets = {
        "sing-box/cf-private-key" = {
          owner = "sing-box";
          group = "sing-box";
        };
      }
      // listToAttrs (
        map (
          client:
          nameValuePair "vless/${client}-uuid" {
            owner = "sing-box";
            group = "sing-box";
          }
        ) cfg.clients
      );
    };

    ${namespace}.services.nginx = {
      enable = true;
      virtualHosts = {
        "datalake.elxreno.com" = {
          locations."/api/v1/stream" = {
            proxyPass = "http://localhost:10000";
            proxyWebsockets = true;
          };
        };
      };
    };

    networking.firewall = {
      allowedUDPPorts = [ 8443 ];
    };

    services.sing-box = {
      enable = true;
      settings = {
        log.level = "debug";
        dns = {
          servers = [
            {
              tag = "local";
              type = "local";
            }
          ];
          final = "local";
          strategy = "prefer_ipv6";
        };

        outbounds = [
          {
            tag = "direct-out";
            type = "direct";
          }
        ];

        inbounds = [
          {
            tag = "vless-in";
            type = "vless";

            listen = "::";
            listen_port = 10000;

            users = map (name: {
              inherit name;
              uuid._secret = config.sops.secrets."vless/${name}-uuid".path;
            }) cfg.clients;

            transport = {
              type = "ws";
              path = "/api/v1/stream";
              max_early_data = 2048;
              early_data_header_name = "Sec-WebSocket-Protocol";
              headers.Host = [ "datalake.elxreno.com" ];
            };
          }
          {
            tag = "vless-quic-in";
            type = "vless";

            listen = "::";
            listen_port = 8443;

            users = map (name: {
              inherit name;
              uuid._secret = config.sops.secrets."vless/${name}-uuid".path;
            }) cfg.clients;

            tls = {
              enabled = true;
              server_name = "datalake.elxreno.com";
              certificate_path = "${config.security.acme.certs."datalake.elxreno.com".directory}/cert.pem";
              key_path = "${config.security.acme.certs."datalake.elxreno.com".directory}/key.pem";
            };

            transport = {
              type = "quic";
            };
          }
        ];

        endpoints = [
          {
            type = "wireguard";
            tag = "cloudflare";

            mtu = 1280;
            address = [
              "172.16.0.2/32"
              "2606:4700:110:8949:fed8:2642:a640:c8e1/128"
            ];
            private_key._secret = config.sops.secrets."sing-box/cf-private-key".path;
            peers = [
              {
                address = "engage.cloudflareclient.com";
                port = 2408;
                public_key = "bmXOC+F1FxEMF9dyiK2H5/1SUtzH0JuVo51h2wPfgyo=";
                allowed_ips = [
                  "0.0.0.0/0"
                  "::/0"
                ];
                reserved = [
                  243
                  193
                  236
                ];
              }
            ];
          }
        ];

        route = {
          rules = [
            {
              domain_suffix = [
                ".ru"
              ];
              rule_set = [
                "geoip-ru"
              ];
              action = "reject";
            }
            {
              domain_suffix = [
                ".by"
              ];
              rule_set = [
                "geoip-by"
              ];
              outbound = "cloudflare";
            }
            {
              action = "sniff";
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
            }
            {
              tag = "geoip-by";
              type = "remote";
              format = "binary";
              url = "https://raw.githubusercontent.com/SagerNet/sing-geoip/rule-set/geoip-by.srs";
            }
          ];

          final = "direct-out";
          auto_detect_interface = true;
        };
      };
    };

    users.users.sing-box.extraGroups = [ "acme" ];
  };
}
