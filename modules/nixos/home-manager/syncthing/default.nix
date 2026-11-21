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
    mkOption
    mapAttrs
    concatLists
    mapAttrsToList
    filterAttrs
    ;
  inherit (lib.trivial) mod;
  cfg = config.${namespace}.home-manager.syncthing;

  syncthingUsers = filterAttrs (
    name: u:
    u.isNormalUser
    && (config.home-manager.users.${name}.${namespace}.services.syncthing.enable or false)
  ) config.users.users;
in
{
  options.${namespace}.home-manager.syncthing = {
    enable = mkEnableOption "managing extra stuff for Syncthing" // {
      default = true;
    };
    randomPortIncrement = mkOption {
      description = "Random increment number for ports";
      default = 0;
    };
    ports = mkOption {
      description = "Computed syncthing ports per user.";
      readOnly = true;
      default = mapAttrs (
        _: user:
        let
          inc = mod user.uid 10;
        in
        {
          webUIPort = 8384 + inc;
          announcePort = 21027 + inc + cfg.randomPortIncrement;
          listenPort = 22000 + inc + cfg.randomPortIncrement;
        }
      ) syncthingUsers;
    };
  };

  config = mkIf cfg.enable {
    ${namespace} = {
      ports = lib.concatMapAttrs (user: p: {
        "syncthing-${user}-webui" = p.webUIPort;
        "syncthing-${user}-announce" = p.announcePort;
        "syncthing-${user}-listen" = p.listenPort;
      }) cfg.ports;

      system.networking.firewall.public.syncthing = {
        tcp = mapAttrsToList (_: p: p.listenPort) cfg.ports;
        udp = concatLists (
          mapAttrsToList (_: p: [
            p.announcePort
            p.listenPort
          ]) cfg.ports
        );
      };
    };
  };
}
