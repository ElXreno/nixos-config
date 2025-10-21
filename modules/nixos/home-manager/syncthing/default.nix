{
  config,
  namespace,
  lib,
  ...
}:

let
  inherit (lib)
    mkIf
    mkEnableOption
    mapAttrs
    concatLists
    mapAttrsToList
    ;
  inherit (lib.trivial) mod;
  cfg = config.${namespace}.home-manager.syncthing;

  ports = mapAttrs (
    _: user:
    let
      inc = mod user.uid 10;
    in
    {
      webUIPort = 8384 + inc;
      announcePort = 21027 + inc;
      listenPort = 22000 + inc;
    }
  ) config.${namespace}.user.users;

  tcpPorts = mapAttrsToList (_: ports: ports.listenPort) ports;

  udpPorts = concatLists (
    mapAttrsToList (_: ports: [
      ports.announcePort
      ports.listenPort
    ]) ports
  );
in
{
  options.${namespace}.home-manager.syncthing = {
    enable = mkEnableOption "Whether or not to manage extra stuff for Syncthing." // {
      default = true;
    };
  };

  config = mkIf cfg.enable {
    networking.firewall.allowedTCPPorts = tcpPorts;
    networking.firewall.allowedUDPPorts = udpPorts;

    home-manager = {
      users = mapAttrs (name: ports: {
        services.syncthing = with ports; {
          guiAddress = "127.0.0.1:${toString webUIPort}";

          settings = {
            options = {
              # https://docs.syncthing.net/v2.0.0/users/config#listen-addresses
              listenAddresses = [
                "tcp://:${toString listenPort}"
                "quic://:${toString listenPort}"
              ];
              localAnnounceEnabled = true;
              localAnnouncePort = announcePort;
              localAnnounceMCAddr = "[ff12::${toString webUIPort}]:${toString announcePort}";
            };
          };
        };
      }) ports;
    };
  };
}
