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
          allowedIPs = [ "0.0.0.0/0" "::/0" ];
          endpoint = "141.144.252.150:51820";
          persistentKeepalive = 5;
        }
      ];
    };
    "rtx4d-local" = {
      privateKeyFile = config.sops.secrets."wg/rtx4d-local".path;
      address = [ "10.11.12.2/24" ];
      peers = [
        {
          publicKey = "MzQPQzUFr4aQYD/fdAVvitNXXbRf0LL4pmpal+VNbQ4=";
          allowedIPs = [ "10.11.12.1/32" "192.168.1.0/24" ];
          endpoint = "109.171.24.112:65534";
          persistentKeepalive = 5;
        }
      ];
    };
    "kebab" = {
      privateKeyFile = config.sops.secrets."wg/infinity-kebab".path;
      address = [ "10.0.0.2/24" ];
      peers = [
        {
          publicKey = "L5OFqfoKXuC2H1htUXXzZ2iD+VEfroG55/izZzVjfT8=";
          allowedIPs = [ "0.0.0.0/0" "::/0" ];
          endpoint = "37.27.17.16:21367";
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
    "wg-quick-rtx4d-local" = {
      wantedBy = lib.mkForce [ ];
    };
    "wg-quick-kebab" = {
      wantedBy = lib.mkForce [ ];
    };
  };

  sops.secrets."wg/cloudflare" = { };
  sops.secrets."wg/nl" = { };
  sops.secrets."wg/nl-preshared" = { };
  sops.secrets."wg/rtx4d-local" = { };
  sops.secrets."wg/infinity-kebab" = { };
}
