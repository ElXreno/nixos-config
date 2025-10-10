{
  config,
  lib,
  virtual,
  ...
}:
{
  config = lib.mkIf (!virtual) {
    networking.wg-quick.interfaces = {
      "cloudflare" = {
        privateKeyFile = config.sops.secrets."wg/cloudflare".path;
        address = [
          "172.16.0.2/32"
          "2606:4700:110:84ac:2661:848b:7830:e951/128"
        ];
        mtu = 1280;
        peers = [
          {
            publicKey = "bmXOC+F1FxEMF9dyiK2H5/1SUtzH0JuVo51h2wPfgyo=";
            allowedIPs = [
              "0.0.0.0/0"
              "::/0"
            ];
            endpoint = "162.159.195.1:500";
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
  };
}
