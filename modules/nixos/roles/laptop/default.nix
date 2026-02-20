{
  config,
  namespace,
  lib,
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
        hardware.bluetooth.enable = true;

        nix = {
          daemonCPUSchedPolicy = "idle";
          daemonCPUWeight = 1;
        };
      };
    };

    services.logind.settings.Login.HandleLidSwitch = "ignore";
  };
}
