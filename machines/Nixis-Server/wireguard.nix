{ config, pkgs, lib, ... }:

{
  networking.firewall.allowedUDPPorts = [ 21367 ];

  networking.nat = {
    enable = true;
    externalInterface = "eth0";
    internalInterfaces = [ "wg0" ];
  };

  networking.wg-quick.interfaces."wg0" = {
    privateKeyFile = config.sops.secrets."wg/nixis".path;
    listenPort = 21367;
    address = [ "10.0.0.1" ];
    preUp = "iptables -A POSTROUTING -t nat -j MASQUERADE -s 10.0.0.0/24 -o eth0";
    preDown = "iptables -D POSTROUTING -t nat -j MASQUERADE -s 10.0.0.0/24 -o eth0";
    peers = [
      {
        publicKey = "lIwMys+xNAS1qNm+rYjErNkijcWQ01XazlH2Rl7uHl4=";
        allowedIPs = [ "10.0.0.2/32" ];
      }
    ];
  };

  sops.secrets."wg/nixis" = { };
}
