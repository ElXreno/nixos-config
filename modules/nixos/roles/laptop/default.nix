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
        scx = {
          enable = true;
          scheduler = "scx_layered";
          layered.layers = {
            audio = {
              priority = 5;
              matches = [
                [ { PcommPrefix = "pipewire"; } ]
                [ { PcommPrefix = "wireplumber"; } ]
                [ { PcommPrefix = "bluetoothd"; } ]
                [ { PcommPrefix = ".spotify"; } ]
              ];
              kind.Grouped = {
                util_range = [
                  0.0
                  0.95
                ];
                cpus_range_frac = [
                  0.0
                  1.0
                ];
                preempt = true;
                preempt_first = true;
              };
            };
            compositor = {
              priority = 10;
              matches = [
                [ { PcommPrefix = "niri"; } ]
                [ { PcommPrefix = "kwin_wayland"; } ]
                [ { PcommPrefix = "kwin_x11"; } ]
                [ { CgroupRegex = ".+noctalia.+\\.scope"; } ]
                [ { CgroupRegex = "niri\\.service"; } ]
              ];
              kind.Grouped = {
                util_range = [
                  0.0
                  0.95
                ];
                cpus_range_frac = [
                  0.1
                  0.5
                ];
                weight = 10000;
                preempt = true;
                preempt_first = true;
              };
            };
            minecraft-render = {
              priority = 13;
              matches = [
                [
                  { PcommPrefix = "java"; }
                  { CommPrefix = "Render thread"; }
                ]
                [
                  { PcommPrefix = "java"; }
                  { CommPrefix = "CullThread"; }
                ]
                [
                  { PcommPrefix = "java"; }
                  { CommPrefix = "Chunk Render"; }
                ]
                [
                  { PcommPrefix = "java"; }
                  { CommPrefix = "Flywheel"; }
                ]
                [
                  { PcommPrefix = "java"; }
                  { CommPrefix = "AsyncParticle"; }
                ]
              ];
              kind.Grouped = {
                util_range = [
                  0.0
                  0.95
                ];
                cpus_range_frac = [
                  0.0
                  1.0
                ];
                weight = 5000;
                preempt = true;
                preempt_first = true;
                exclusive = true;
                prev_over_idle_core = true;
                idle_confined = true;
              };
            };
            minecraft-server = {
              priority = 14;
              matches = [
                [
                  { PcommPrefix = "java"; }
                  { CommPrefix = "Server thread"; }
                ]
              ];
              kind.Grouped = {
                util_range = [
                  0.0
                  0.95
                ];
                cpus_range_frac = [
                  0.0
                  1.0
                ];
                weight = 4000;
                preempt = true;
                preempt_first = true;
                prev_over_idle_core = true;
                idle_confined = true;
              };
            };
            minecraft-gc = {
              priority = 15;
              matches = [
                [
                  { PcommPrefix = "java"; }
                  { CommPrefix = "GC Thread#"; }
                ]
                [
                  { PcommPrefix = "java"; }
                  { CommPrefix = "G1 "; }
                ]
              ];
              kind.Grouped = {
                util_range = [
                  0.0
                  0.95
                ];
                cpus_range_frac = [
                  0.0
                  1.0
                ];
              };
            };
            minecraft-jit = {
              priority = 16;
              matches = [
                [
                  { PcommPrefix = "java"; }
                  { CommPrefix = "C1 Compiler"; }
                ]
                [
                  { PcommPrefix = "java"; }
                  { CommPrefix = "C2 Compiler"; }
                ]
                [
                  { PcommPrefix = "java"; }
                  { CommPrefix = "JVMCI-native"; }
                ]
              ];
              kind.Open = {
                weight = 10;
              };
            };
            games = {
              priority = 17;
              matches = [
                [ { CgroupRegex = ".+(TaterClient|prismlauncher).+\\.scope"; } ]
              ];
              kind.Grouped = {
                util_range = [
                  0.0
                  0.95
                ];
                cpus_range_frac = [
                  0.0
                  1.0
                ];
                weight = 1000;
                preempt = true;
                preempt_first = true;
                exclusive = true;
              };
            };
            # Chunk-gen / async worker pool: deprioritized below `games` and
            # `interactive` so the Render and Server threads can preempt them
            # when the player flies through new chunks. Vanilla MC's
            # Util.backgroundExecutor and Sodium's ChunkBuilder both spawn
            # Worker-Main-N threads pegged at availableProcessors()-1.
            minecraft-workers = {
              priority = 26;
              matches = [
                [
                  { PcommPrefix = "java"; }
                  { CommPrefix = "Worker-Main"; }
                ]
                [
                  { PcommPrefix = "java"; }
                  { CommPrefix = "ChunkBuilder"; }
                ]
              ];
              kind.Open = {
                weight = 100;
              };
            };
            interactive = {
              priority = 30;
              matches = [
                [ { CgroupRegex = "app-niri-"; } ]
              ];
              kind.Grouped = {
                util_range = [
                  0.0
                  0.95
                ];
                cpus_range_frac = [
                  0.0
                  1.0
                ];
                weight = 1000;
                preempt = true;
                preempt_first = true;
              };
            };
          };
        };
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

    services.logind.settings.Login = {
      HandleLidSwitch = "ignore";
      HandleSuspendKey = "ignore";
      HandleSuspendKeyLongPress = "ignore";
    };
  };
}
