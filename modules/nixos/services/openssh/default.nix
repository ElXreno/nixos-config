{
  config,
  namespace,
  lib,
  ...
}:

let
  inherit (lib) mkIf mkEnableOption;
  cfg = config.${namespace}.services.openssh;
in
{
  options.${namespace}.services.openssh = {
    enable = mkEnableOption "managing openssh";
  };

  config = mkIf cfg.enable {
    ${namespace}.ports.openssh = lib.head config.services.openssh.ports;

    services.openssh = {
      openFirewall = false;
      settings.KbdInteractiveAuthentication = false;
    };
  };
}
