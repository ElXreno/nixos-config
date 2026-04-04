{
  config,
  lib,
  pkgs,
  virtual,
  ...
}:
let
  wireguardPort = 17231;
  ethInterface = "ens3";
in
{
  config = lib.mkIf (!virtual) {
    networking = {
      firewall.allowedUDPPorts = [ wireguardPort ];

      nat = {
        enable = true;
        externalInterface = ethInterface;
        internalInterfaces = [ "wg0" ];
      };

      wg-quick.interfaces."wg0" = {
        privateKeyFile = config.clan.core.vars.generators."wg-destroyer".files.privateKey.path;
        listenPort = wireguardPort;
        address = [ "10.100.0.1" ];
        preUp = "iptables -A POSTROUTING -t nat -j MASQUERADE -s 10.100.0.0/24 -o ${ethInterface}";
        preDown = "iptables -D POSTROUTING -t nat -j MASQUERADE -s 10.100.0.0/24 -o ${ethInterface}";
        peers = [
          # Router
          {
            publicKey = "bnnwHNptilPEQQOkaXs8SbNRE+hG0wlQwt5MMoeq2jg=";
            presharedKeyFile = config.clan.core.vars.generators."wg-preshared-destroyer-router".files.key.path;
            allowedIPs = [ "10.100.0.2/32" ];
          }
        ];
      };
    };

    clan.core.vars.generators."wg-destroyer" = {
      runtimeInputs = [ pkgs.wireguard-tools ];
      files.privateKey.secret = true;
      script = ''
        wg genkey > "$out/privateKey"
      '';
    };

    clan.core.vars.generators."wg-preshared-destroyer-router" = {
      runtimeInputs = [ pkgs.wireguard-tools ];
      files.key.secret = true;
      script = ''
        wg genpsk > "$out/key"
      '';
    };
  };
}
