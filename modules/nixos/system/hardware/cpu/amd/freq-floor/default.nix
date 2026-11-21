{
  config,
  namespace,
  lib,
  pkgs,
  ...
}:
let
  inherit (lib)
    mkIf
    mkEnableOption
    mkOption
    optionalAttrs
    types
    ;
  cfg = config.${namespace}.system.hardware.cpu.amd.freq-floor;
  ppd = config.services.power-profiles-daemon.enable;
  ppdState = "/var/lib/power-profiles-daemon/state.ini";
in
{
  options.${namespace}.system.hardware.cpu.amd.freq-floor = {
    enable = mkEnableOption "raising the cpufreq floor while on AC";

    acFreq = mkOption {
      type = types.ints.positive;
      default = 3500000;
      description = ''
        scaling_min_freq in kHz applied to every policy while on AC power.

        A core that sleeps between bursts restarts from a low operating point,
        which costs roughly 8% on latency-sensitive single-threaded work. Raising
        the floor recovers most of it. On battery the driver's own
        lowest_nonlinear_freq is restored, since the floor costs about 0.3 W at
        idle.
      '';
    };
  };

  config = mkIf cfg.enable {
    services.power-profiles-daemon.package = mkIf ppd (
      pkgs.power-profiles-daemon.overrideAttrs (prevAttrs: {
        postPatch = (prevAttrs.postPatch or "") + ''
          substituteInPlace src/ppd-driver-amd-pstate.c \
            --replace-fail 'if (g_file_test (min_freq_path, G_FILE_TEST_EXISTS)) {' 'if (FALSE) {'
          substituteInPlace tests/integration_test.py \
            --replace-fail 'def test_amd_pstate_min_freq' 'def skip_amd_pstate_min_freq'
        '';
      })
    );

    systemd.services.cpu-freq-floor = {
      description = "Raise the cpufreq floor while on AC power";
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
      # A power transition fires udev plus one or more profile changes back to
      # back, easily exceeding the default 5-starts-per-10s limit. Hitting it
      # puts the .path unit into failed state and the floor stops being
      # re-applied at all. The unit is idempotent and runs in ~40 ms.
      unitConfig.StartLimitIntervalSec = 0;
      script = ''
        if ${config.systemd.package}/bin/systemd-ac-power; then
          want=${toString cfg.acFreq}
        else
          want=""
        fi

        apply() {
          for d in /sys/devices/system/cpu/cpu[0-9]*/cpufreq; do
            [ -w "$d/scaling_min_freq" ] || continue

            floor=$want
            if [ -z "$floor" ]; then
              # on battery hand the policy back to the driver's own floor;
              # never fall back to cpuinfo_min_freq, that is not the default
              floor=$(cat "$d/amd_pstate_lowest_nonlinear_freq" 2>/dev/null) || continue
            fi
            [ -n "$floor" ] || continue

            echo "$floor" > "$d/scaling_min_freq" || true
          done
        }

        # power-profiles-daemon rewrites scaling_min_freq on every profile
        # change, so re-apply once if something raced us.
        apply
        got=$(cat /sys/devices/system/cpu/cpu0/cpufreq/scaling_min_freq 2>/dev/null || echo "")
        if [ -n "$want" ] && [ "$got" != "$want" ]; then
          sleep 1
          apply
        fi
      '';
    };

    # PPD rewrites its state file on every profile change (including the
    # automatic battery_aware switch), which is our cue to put the floor back.
    systemd.paths = optionalAttrs ppd {
      cpu-freq-floor = {
        description = "Re-apply the cpufreq floor after a power profile change";
        wantedBy = [ "multi-user.target" ];
        pathConfig = {
          PathModified = ppdState;
          Unit = "cpu-freq-floor.service";
        };
      };
    };

    services.udev.extraRules = ''
      ACTION=="change", SUBSYSTEM=="power_supply", ATTR{type}=="Mains", TAG+="systemd", ENV{SYSTEMD_WANTS}+="cpu-freq-floor.service"
    '';
  };
}
