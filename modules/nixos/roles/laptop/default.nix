{
  config,
  namespace,
  lib,
  pkgs,
  ...
}:

let
  inherit (lib) mkIf mkEnableOption;
  cfg = config.${namespace}.roles.laptop;
in
{
  options.${namespace}.roles.laptop = {
    enable = mkEnableOption "Whether or not to enable laptop configuration.";
  };

  config = mkIf cfg.enable {
    ${namespace} = {
      roles.common.enable = true;

      common-packages.enable = true;

      services = {
        pipewire.enable = true;

        printing.enable = true;
      };

      programs = {
        adb.enable = true;
      };

      system = {
        boot.kernel.packages = pkgs.linuxPackages_xanmod_latest;
        hardware.bluetooth.enable = true;
      };
    };

    services.logind.settings.Login.HandleLidSwitch = "ignore";
  };
}
