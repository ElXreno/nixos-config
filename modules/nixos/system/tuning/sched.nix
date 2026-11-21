{
  config,
  namespace,
  lib,
  ...
}:
let
  inherit (lib) mkIf mkEnableOption;
  cfg = config.${namespace}.system.tuning.sched;

  features = "/sys/kernel/debug/sched/features";
in
{
  options.${namespace}.system.tuning.sched = {
    enable = mkEnableOption "keeping sync-woken tasks on their own idle CPU";
  };

  config = mkIf cfg.enable {
    systemd.services.sched-features = {
      description = "Keep sync-woken tasks on their own idle CPU";
      wantedBy = [ "multi-user.target" ];
      unitConfig.ConditionPathExists = features;
      serviceConfig = {
        Type = "oneshot";
        RemainAfterExit = true;
      };
      script = "echo NO_WA_IDLE > ${features}";
      postStop = "echo WA_IDLE > ${features}";
    };
  };
}
