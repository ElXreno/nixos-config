{ config, ... }: {
  networking.nat = {
    enable = true;
    enableIPv6 = true;
    externalInterface = "ens3";
    internalInterfaces = [ "wgvpn" ];
  };

  networking.firewall.allowedUDPPorts = [ 55553 ];
  networking.wg-quick.interfaces."wgvpn" = {
    privateKeyFile = config.sops.secrets."wg/romeo".path;
    listenPort = 55553;
    address = [ "10.0.0.1" ];
    preUp =
      "iptables -A POSTROUTING -t nat -j MASQUERADE -s 10.0.0.0/24 -o ens3";
    preDown =
      "iptables -D POSTROUTING -t nat -j MASQUERADE -s 10.0.0.0/24 -o ens3";
    peers = [
      {
        publicKey = "5sg4eTXAkFl1AwOgn0h1moJ1/MmxjYgzDe0p5krSZnI=";
        allowedIPs = [ "10.0.0.2/32" ];
      }
      {
        publicKey = "iY/8FM2sLy8t28o4yjtlpOxSnftTVAU8xl5+OJTEbT8=";
        allowedIPs = [ "10.0.0.3/32" ];
      }
    ];
  };

  sops.secrets."wg/romeo" = { };
}
