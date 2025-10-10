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
          "fd01:5ca1:ab1e:8ac1:1850:1199:777a:18df/128"
        ];
        dns = [ "1.1.1.1" ];
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
      "nl" = {
        privateKeyFile = config.sops.secrets."wg/nl".path;
        address = [ "10.6.0.7/24" ];
        dns = [ "1.1.1.1" ];
        peers = [
          {
            publicKey = "RWdtN7Aguq1Z6l602+1LZowlrVRZMl350TZO24w6eWM=";
            presharedKeyFile = config.sops.secrets."wg/nl-preshared".path;
            allowedIPs = [
              "0.0.0.0/0"
              "::/0"
            ];
            endpoint = "141.144.252.150:51820";
            persistentKeepalive = 5;
          }
        ];
      };
    };

    systemd.services = {
      "wg-quick-cloudflare" = {
        wantedBy = lib.mkForce [ ];
      };
      "wg-quick-nl" = {
        wantedBy = lib.mkForce [ ];
      };
    };

    sops.secrets."wg/cloudflare" = { };
    sops.secrets."wg/nl" = { };
    sops.secrets."wg/nl-preshared" = { };
  };
}
