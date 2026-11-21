{
  config,
  osConfig,
  namespace,
  lib,
  ...
}:

let
  inherit (lib)
    attrNames
    elem
    findFirst
    mapAttrs
    mkIf
    mkEnableOption
    optionalAttrs
    ;
  cfg = config.${namespace}.services.syncthing;
  inherit (osConfig.${namespace}.home-manager.syncthing.ports.${config.home.username})
    webUIPort
    announcePort
    listenPort
    ;
  topology = import ./_topology.nix;
  inherit (osConfig.networking) hostName;
  self = findFirst (
    name: elem hostName (topology.devices.${name}.hosts or [ name ])
  ) (throw "syncthing: no device for ${hostName} in _topology.nix") (attrNames topology.devices);
  peers = removeAttrs topology.devices [ self ];
in
{
  options.${namespace}.services.syncthing.enable = mkEnableOption "managing syncthing";

  config = mkIf cfg.enable {
    services.syncthing = {
      enable = true;

      overrideDevices = true;
      overrideFolders = true;

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
        devices = mapAttrs (_: d: { inherit (d) id; }) peers;
        folders = mapAttrs (
          _: id:
          {
            inherit id;
            devices = attrNames peers;
          }
          // optionalAttrs (topology.devices.${self}.receiveOnly or false) { type = "receiveonly"; }
        ) topology.folders;
      };
    };
  };
}
