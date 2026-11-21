{
  config,
  namespace,
  lib,
  pkgs,
  ...
}:

let
  inherit (lib) mkIf mkEnableOption;
  cfg = config.${namespace}.services.printing;
in
{
  options.${namespace}.services.printing = {
    enable = mkEnableOption "managing printing";
  };

  config = mkIf cfg.enable {
    services = {
      avahi = {
        enable = true;
        nssmdns4 = true;
      };
      printing = {
        enable = true;
        drivers = with pkgs; [ gutenprint ];
      };
    };
  };
}
