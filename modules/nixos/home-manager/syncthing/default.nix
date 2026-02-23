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

  normalUsers = filterAttrs (_: u: u.isNormalUser) config.users.users;
in
{
  options.${namespace}.home-manager.syncthing = {
    enable = mkEnableOption "Whether or not to manage extra stuff for Syncthing." // {
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
      ) normalUsers;
    };
  };

  config = mkIf cfg.enable {
    networking.firewall.allowedTCPPorts = mapAttrsToList (_: p: p.listenPort) cfg.ports;
    networking.firewall.allowedUDPPorts = concatLists (
      mapAttrsToList (_: p: [
        p.announcePort
        p.listenPort
      ]) cfg.ports
    );
  };
}
