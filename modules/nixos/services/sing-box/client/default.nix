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
        "xray/uuid" = { };
        "xray/public-key" = { };
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
              detour = "direct";
            }
          ];

          final = "local-dns"; # Route to the dnscrypt-proxy
          disable_cache = false;
          disable_expire = false;
        };

        inbounds = [
          {
            type = "tun";
            domain_strategy = "prefer_ipv4";
            address = [ "172.16.0.1/30" ];
            stack = "gvisor";
            auto_route = true;
            strict_route = true;
          }
        ];

        outbounds = [
          {
            type = "vless";
            tag = "proxy";
            server = "74.119.195.240";
            server_port = 443;
            uuid._secret = config.sops.secrets."xray/uuid".path;
            flow = "xtls-rprx-vision";
            tls = {
              enabled = true;
              server_name = "elxreno.com";
              reality = {
                enabled = true;
                public_key._secret = config.sops.secrets."xray/public-key".path;
                short_id = "881d49987e1f93e7";
              };
              utls = {
                enabled = true;
                fingerprint = "chrome";
              };
            };
          }
          {
            type = "direct";
            tag = "direct";
          }
        ];

        route = {
          auto_detect_interface = true;

          rules = [
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
              download_detour = "proxy";
            }
            {
              tag = "geoip-by";
              type = "remote";
              format = "binary";
              url = "https://raw.githubusercontent.com/SagerNet/sing-geoip/rule-set/geoip-by.srs";
              download_detour = "proxy";
            }
          ];

          final = "proxy";
        };

        experimental.cache_file.enabled = true;
      };
    };

    systemd.services.sing-box.wantedBy = mkIf (!cfg.autostart) (lib.mkForce [ ]);
  };
}
