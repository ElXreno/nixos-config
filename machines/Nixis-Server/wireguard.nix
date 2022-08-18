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
    address = [ "10.0.0.4" ];
    preUp = "iptables -A POSTROUTING -t nat -j MASQUERADE -s 10.0.0.0/24 -o ens3";
    preDown = "iptables -D POSTROUTING -t nat -j MASQUERADE -s 10.0.0.0/24 -o ens3";
    peers = [
      {
        publicKey = "XPGJyDu4H1VuchnsEqjrlImTW/tpU6YV9eKKd7amCX8=";
        allowedIPs = [ "10.0.0.0/24" "10.0.0.1/32" ];
        endpoint = "158.101.193.16:17532";
      }
      {
        publicKey = "06EDbz1mPE6L22oFJWVHqxApMGwZ9Hao6nqTB2JOuj4=";
        allowedIPs = [ "10.0.0.2/32" ];
      }
      {
        publicKey = "l7YuhmxvVg4S3j62rrJCKi6B27eb7VxX3iAenGVqQjE=";
        allowedIPs = [ "10.0.0.3/32" ];
      }
      {
        publicKey = "UcTTHXYKevw+ZMCQiadeYOF0xYGZIXJFjzUhxeMfx1Y=";
        allowedIPs = [ "10.0.0.5/32" ];
      }
      {
        publicKey = "o8PfXqjSJMRkes9IiYMe6V+m15FqEUbIWX/9T9c+mBc=";
        allowedIPs = [ "10.0.0.6/32" ];
      }
    ];
  };

  sops.secrets."wg/nixis" = { };
}
