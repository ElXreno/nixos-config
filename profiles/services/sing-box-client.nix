{ config, ... }:
{
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
            address = "127.0.0.1";
            detour = "direct";
          }
        ];

        final = "local-dns"; # Route to the dnscrypt-proxy2
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
          sniff = true;
        }
      ];

      outbounds = [
        {
          type = "vless";
          tag = "proxy";
          server = "elxreno.com";
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
        {
          type = "block";
          tag = "block";
        }
        {
          type = "dns";
          tag = "dns-out";
        }

      ];

      route = {
        auto_detect_interface = true;

        rules = [
          {
            protocol = "dns";
            outbound = "dns-out";
          }
          {
            protocol = "bittorrent";
            outbound = "block";
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

}
