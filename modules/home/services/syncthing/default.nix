{
  config,
  osConfig,
  namespace,
  lib,
  ...
}:

let
  inherit (lib)
    mkIf
    mkEnableOption
    mkOption
    types
    ;
  cfg = config.${namespace}.services.syncthing;
  inherit (osConfig.${namespace}.home-manager.syncthing.ports.${config.home.username})
    webUIPort
    announcePort
    listenPort
    ;
in
{
  options.${namespace}.services.syncthing = {
    enable = mkEnableOption "Whether or not to manage syncthing.";
    settings = mkOption {
      type = types.raw;
      default = { };
    };
  };

  config = mkIf cfg.enable {
    services.syncthing = {
      enable = true;

      overrideDevices = true;
      overrideFolders = true;

      guiAddress = "127.0.0.1:${toString webUIPort}";

      settings = lib.recursiveUpdate {
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
      } cfg.settings;
    };
  };
}
