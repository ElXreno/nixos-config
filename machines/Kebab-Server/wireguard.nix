{ config, ... }:

{
  networking.firewall.allowedUDPPorts = [ 21367 ];

  networking.nat = {
    enable = true;
    externalInterface = "enp1s0";
    internalInterfaces = [ "wg0" ];
  };

  networking.wg-quick.interfaces."wg0" = {
    privateKeyFile = config.sops.secrets."wg/kebab".path;
    listenPort = 21367;
    address = [ "10.0.0.1" ];
    preUp = "iptables -A POSTROUTING -t nat -j MASQUERADE -s 10.0.0.0/24 -o enp1s0";
    preDown = "iptables -D POSTROUTING -t nat -j MASQUERADE -s 10.0.0.0/24 -o enp1s0";
    peers = [
      {
        publicKey = "lIwMys+xNAS1qNm+rYjErNkijcWQ01XazlH2Rl7uHl4=";
        allowedIPs = [ "10.0.0.2/32" ];
      }
      {
        publicKey = "JcOYsdD3A0r7X8kYKNrj+Z/ZD7/9M693rQiRyP39iy8=";
        allowedIPs = [ "10.0.0.3/32" ];
      }
    ];
  };

  sops.secrets."wg/kebab" = { };
}
