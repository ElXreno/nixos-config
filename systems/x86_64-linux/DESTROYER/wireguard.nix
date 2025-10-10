{
  config,
  lib,
  virtual,
  ...
}:
let
  wireguardPort = 17231;
  ethInterface = "ens3";
in
{
  config = lib.mkIf (!virtual) {
    networking.firewall.allowedUDPPorts = [ wireguardPort ];

    networking.nat = {
      enable = true;
      externalInterface = ethInterface;
      internalInterfaces = [ "wg0" ];
    };

    networking.wg-quick.interfaces."wg0" = {
      privateKeyFile = config.sops.secrets."wg/destroyer".path;
      listenPort = wireguardPort;
      address = [ "10.100.0.1" ];
      preUp = "iptables -A POSTROUTING -t nat -j MASQUERADE -s 10.100.0.0/24 -o ${ethInterface}";
      preDown = "iptables -D POSTROUTING -t nat -j MASQUERADE -s 10.100.0.0/24 -o ${ethInterface}";
      peers = [
        # Router
        {
          publicKey = "WutIrPxgNW7tqKsK/X9sC/+N/K+FimwNg6XqXzNPGHY=";
          presharedKeyFile = config.sops.secrets."wg-preshared/destroyer-router".path;
          allowedIPs = [ "10.100.0.2/32" ];
        }
      ];
    };

    sops.secrets."wg/destroyer" = { };
    sops.secrets."wg-preshared/destroyer-router" = { };
  };
}
