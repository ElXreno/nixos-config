{
  config,
  namespace,
  lib,
  ...
}:

let
  inherit (lib) mkIf mkEnableOption;
  cfg = config.${namespace}.services.syncthing;
in
{
  options.${namespace}.services.syncthing = {
    enable = mkEnableOption "Whether or not to manage syncthing.";
  };

  config = mkIf cfg.enable {
    services.syncthing.enable = true;

    # TODO: enable ports system-wire, calculate them with uid
    # networking.firewall = {
    #   allowedTCPPorts = [ 22000 ]; # TCP based sync protocol traffic
    #   allowedUDPPorts = [
    #     22000
    #     21027
    #   ]; # QUIC based sync protocol traffic & for discovery broadcasts
    # };
  };
}
