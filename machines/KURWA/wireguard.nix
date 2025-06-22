{ config, lib, ... }:

{
  networking.wg-quick.interfaces = {
    "cloudflare" = {
      privateKeyFile = config.sops.secrets."wg/cloudflare".path;
      address = [
        "172.16.0.2/32"
        "2606:4700:110:8c5b:8b41:f688:782b:e121/128"
      ];
      mtu = 1280;
      peers = [
        {
          publicKey = "bmXOC+F1FxEMF9dyiK2H5/1SUtzH0JuVo51h2wPfgyo=";
          allowedIPs = [
            "0.0.0.0/0"
            "::/0"
          ];
          endpoint = "engage.cloudflareclient.com:2408";
          persistentKeepalive = 5;
        }
      ];
    };
  };

  systemd.services = {
    "wg-quick-cloudflare" = {
      wantedBy = lib.mkForce [ ];
    };
  };

  sops.secrets."wg/cloudflare" = { };
}
