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
    networking.firewall = {
      allowedTCPPorts = [
        80
        443
      ];
      allowedUDPPorts = [ 443 ];
    };

    services.xray = {
      enable = true;

      settingsFile = config.sops.templates."xray-config.json".path;
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
                  "::/0"
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
                    # Extra
                    "f6be054247adb453"
                    "d6d49e047a490df3"
                    "03628ddf48bc0a82"
                    "10ec2a03760330a4"
                    "16f9f0c3735d399b"
                    "293009144e59aad8"
                    "9607c2cd15f9f063"
                    "d27b5ef3b63c4dac"
                    "8456cbb15fc0bd43"
                    "838e1d74d081b90e"
                    "99925cf5d039d11c"
                    "6e83154006203520"
                    "71addeb5b7f461d0"
                    "d1a6741e735e6224"
                    "a3cccbf0fbb148f9"
                    "3cfee2253ac620b5"
                    "4378d56ce7875205"
                    "7cda3216b469fd92"
                    "80d331e13514877c"
                    "6fbbc5ff88c916ab"
                    "ad41a9911433d1b0"
                    "69d44b1b9be157b6"
                    "e42185c9810adbcd"
                    "a0187e8d3b697d71"
                    "77ad5a39b5aa011c"
                    "62d9de547f196efb"
                    "e5c99aa8d811ee41"
                    "64033c1bc7d44c87"
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
