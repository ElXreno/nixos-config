{ config, ... }:
{
  networking.firewall.allowedUDPPorts = [ 17532 ];

  networking.nat = {
    enable = true;
    externalInterface = "enp0s3";
    internalInterfaces = [ "wg0" ];
  };

  networking.wg-quick.interfaces."wg0" = {
    privateKeyFile = config.sops.secrets."wg/noxer".path;
    listenPort = 17532;
    address = [ "10.0.0.1" ];
    preUp = "iptables -A POSTROUTING -t nat -j MASQUERADE -s 10.0.0.0/24 -o enp0s3";
    preDown = "iptables -D POSTROUTING -t nat -j MASQUERADE -s 10.0.0.0/24 -o enp0s3";
    peers = [
      {
        publicKey = "06EDbz1mPE6L22oFJWVHqxApMGwZ9Hao6nqTB2JOuj4=";
        allowedIPs = [ "10.0.0.2/32" ];
      }
      {
        publicKey = "l7YuhmxvVg4S3j62rrJCKi6B27eb7VxX3iAenGVqQjE=";
        allowedIPs = [ "10.0.0.3/32" ];
      }
      {
        publicKey = "L5OFqfoKXuC2H1htUXXzZ2iD+VEfroG55/izZzVjfT8=";
        allowedIPs = [ "10.0.0.4/32" ];
        endpoint = "104.248.88.168:21367";
      }
      {
        publicKey = "UcTTHXYKevw+ZMCQiadeYOF0xYGZIXJFjzUhxeMfx1Y=";
        allowedIPs = [ "10.0.0.5/32" ];
      }
      {
        publicKey = "o8PfXqjSJMRkes9IiYMe6V+m15FqEUbIWX/9T9c+mBc=";
        allowedIPs = [ "10.0.0.6/32" ];
      }
      {
        publicKey = "0Jz0RuSGHUhySe69aV7TvIl9rbr0bKStVreuajS4wlo=";
        allowedIPs = [ "10.0.0.7/32" ];
      }
    ];
  };

  sops.secrets."wg/noxer" = { };
}
