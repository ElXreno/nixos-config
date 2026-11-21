{
  config,
  namespace,
  lib,
  ...
}:

let
  inherit (lib) mkIf mkDefault mkEnableOption;
  cfg = config.${namespace}.roles.laptop;
in
{
  options.${namespace}.roles.laptop = {
    enable = mkEnableOption "laptop configuration";
  };

  config = mkIf cfg.enable {
    ${namespace} = {
      roles.common.enable = true;

      tailscale.tags = [ "tag:laptop" ];

      programs.partition-manager.enable = true;

      services = {
        pipewire.enable = true;
      };

      system = {
        hardware.bluetooth.enable = true;

        boot.lowLatency = mkDefault true;

        tuning = {
          profile = mkDefault "laptop";
          sched.enable = mkDefault true;
        };
      };
    };

    environment.systemPackages = [ config.boot.kernelPackages.cpupower ];

    services.logind.settings.Login = {
      HandleLidSwitch = "ignore";
      HandleSuspendKey = "ignore";
      HandleSuspendKeyLongPress = "ignore";
    };
  };
}
