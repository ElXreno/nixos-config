{
  config,
  namespace,
  lib,
  ...
}:

let
  inherit (lib) mkIf mkEnableOption;
  cfg = config.${namespace}.services.fail2ban;
in
{
  options.${namespace}.services.fail2ban = {
    enable = mkEnableOption "Whether or not to manage fail2ban.";
  };

  config = mkIf cfg.enable {
    services.fail2ban = {
      enable = true;

      bantime = "3h";
      bantime-increment = {
        enable = true;
        rndtime = "15m";
      };
    };
  };
}
