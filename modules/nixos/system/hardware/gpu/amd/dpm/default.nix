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
    types
    ;
  cfg = config.${namespace}.system.hardware.gpu.amd.dpm;
in
{
  options.${namespace}.system.hardware.gpu.amd.dpm = {
    enable = mkEnableOption "switching amdgpu performance level on AC power";

    acLevel = mkOption {
      type = types.str;
      default = "high";
      description = "power_dpm_force_performance_level applied while on AC.";
    };

    dcLevel = mkOption {
      type = types.str;
      default = "auto";
      description = "power_dpm_force_performance_level applied while on battery.";
    };
  };

  config = mkIf cfg.enable {
    systemd.services.amdgpu-dpm = {
      description = "Set AMD GPU performance level based on AC power";
      after = [
        "systemd-suspend.service"
        "systemd-hibernate.service"
        "systemd-hybrid-sleep.service"
        "systemd-suspend-then-hibernate.service"
      ];
      wantedBy = [
        "multi-user.target"
        "systemd-suspend.service"
        "systemd-hibernate.service"
        "systemd-hybrid-sleep.service"
        "systemd-suspend-then-hibernate.service"
      ];
      serviceConfig.Type = "oneshot";
      script = ''
        if ${config.systemd.package}/bin/systemd-ac-power; then
          level=${cfg.acLevel}
        else
          level=${cfg.dcLevel}
        fi
        for f in /sys/bus/pci/drivers/amdgpu/*/power_dpm_force_performance_level; do
          [ -w "$f" ] && echo "$level" > "$f" || true
        done
      '';
    };

    services.udev.extraRules = ''
      ACTION=="change", SUBSYSTEM=="power_supply", ATTR{type}=="Mains", TAG+="systemd", ENV{SYSTEMD_WANTS}+="amdgpu-dpm.service"
    '';
  };
}
