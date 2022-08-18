{ config, lib, ... }:

{
  networking.wg-quick.interfaces = {
    "cloudflare" = {
      privateKeyFile = config.sops.secrets."wg/cloudflare".path;
      address = [ "172.16.0.2/32" "fd01:5ca1:ab1e:8ac1:1850:1199:777a:18df/128" ];
      dns = [ "1.1.1.1" ];
      peers = [
        {
          publicKey = "bmXOC+F1FxEMF9dyiK2H5/1SUtzH0JuVo51h2wPfgyo=";
          allowedIPs = [ "0.0.0.0/0" "::/0" ];
          endpoint = "engage.cloudflareclient.com:2408";
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
