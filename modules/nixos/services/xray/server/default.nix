{
  config,
  namespace,
  lib,
  ...
}:

let
  inherit (lib) mkIf mkEnableOption;
  cfg = config.${namespace}.services.xray.server;
in
{
  options.${namespace}.services.xray.server = {
    enable = mkEnableOption "Whether or not to manage xray server.";
  };

  config = mkIf cfg.enable {
    services.xray = {
      enable = true;

      settingsFile = config.sops.templates."xray-config.json".path;
    };

    services.nginx = {
      defaultListen = [
        {
          addr = "0.0.0.0";
          port = 8443;
          proxyProtocol = true;
          ssl = true;
        }
        {
          addr = "0.0.0.0";
          port = 80;
          ssl = false;
        }
      ];
    };

    sops = {
      secrets = {
        "xray/uuid" = { };
        "xray/private-key" = { };
        "xray/cf-private-key" = { };
      };

      templates."xray-config.json" = {
        restartUnits = [ "xray.service" ];
        content = builtins.toJSON {
          log = {
            loglevel = "debug";
          };

          routing = {
            domainStrategy = "IPIfNonMatch";
            rules = [
              {
                type = "field";
                outboundTag = "block";
                protocol = [ "bittorrent" ];
              }
              {
                type = "field";
                domain = [
                  "ru"
                  "by"
                ];
                ip = [
                  "geoip:by"
                  "geoip:ru"
                ];
                outboundTag = "cloudflare";
              }
              {
                type = "field";
                ip = [
                  "geoip:private"
                ];
                outboundTag = "block";
              }
            ];
          };

          inbounds = [
            {
              port = 443;
              listen = "0.0.0.0";
              protocol = "vless";

              settings = {
                clients = [
                  {
                    id = config.sops.placeholder."xray/uuid";
                    flow = "xtls-rprx-vision";
                  }
                ];
                decryption = "none";
                fallbacks = [
                  {
                    dest = 8443;
                    xver = 1;
                  }
                ];
              };

              streamSettings = {
                network = "tcp";
                security = "reality";

                realitySettings = {
                  dest = "www.apple.com:443";
                  xver = 0;
                  serverNames = [
                    "apple.com"
                    "www.apple.com"
                  ];
                  privateKey = config.sops.placeholder."xray/private-key";
                  shortIds = [
                    "881d49987e1f93e7"
                    "e7ce83e027ce12de"
                    "7e7ba18f119fdac5"
                  ];
                };
              };

              sniffing = {
                enabled = true;
                destOverride = [
                  "http"
                  "tls"
                  "quic"
                ];
                routeOnly = true;
              };
            }
          ];

          outbounds = [
            {
              protocol = "freedom";
              tag = "out";
            }
            {
              protocol = "wireguard";
              settings = {
                secretKey = config.sops.placeholder."xray/cf-private-key";
                address = [
                  "172.16.0.2/32"
                  "2606:4700:110:8949:fed8:2642:a640:c8e1/128"
                ];
                peers = [
                  {
                    publicKey = "bmXOC+F1FxEMF9dyiK2H5/1SUtzH0JuVo51h2wPfgyo=";
                    allowedIPs = [
                      "0.0.0.0/0"
                      "::/0"
                    ];
                    endpoint = "engage.cloudflareclient.com:2408";
                  }
                ];
                reserved = [
                  243
                  193
                  236
                ];
                mtu = 1280;
              };
              tag = "cloudflare";
            }
            {
              protocol = "blackhole";
              tag = "block";
            }
          ];
        };
      };
    };
  };
}
