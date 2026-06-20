{
  config,
  inputs,
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
    types
    ;
  inherit (inputs.nix-minecraft.lib) collectFilesAt;
  cfg = config.${namespace}.services.minecraft.atm10-sky;

  modpack = pkgs.${namespace}.atm10-sky-modpack;

  serverPackage = modpack.serverPackage.override {
    jre_headless = pkgs.graalvmPackages.graalvm-oracle;
  };

  jvmOpts = lib.concatStringsSep " " [
    "-Xmx12G"
    "-Xms12G"
    "-XX:+UnlockExperimentalVMOptions"
    "-XX:+UnlockDiagnosticVMOptions"
    "-XX:+AlwaysPreTouch"
    "-XX:+UseTransparentHugePages"
    "-XX:+DisableExplicitGC"
    "-XX:+UseStringDeduplication"
    "-XX:+UseCriticalJavaThreadPriority"
    "-XX:+AlwaysActAsServerClassMachine"
    "-XX:+DebugNonSafepoints"
    "-XX:+UseCompactObjectHeaders"
    "-XX:+TrustFinalNonStaticFields"
    "-XX:CICompilerCount=6"
    "-XX:AllocatePrefetchStyle=3"
    "-XX:-DontCompileHugeMethods"
    "-XX:ReservedCodeCacheSize=400M"
    "-XX:NonNMethodCodeHeapSize=12M"
    "-XX:ProfiledCodeHeapSize=194M"
    "-XX:NonProfiledCodeHeapSize=194M"
    "-XX:+UseVectorStubs"
    "-XX:TrimNativeHeapInterval=5000"
    "-XX:G1NewSizePercent=40"
    "-XX:G1MaxNewSizePercent=50"
    "-XX:G1HeapRegionSize=16M"
    "-XX:G1ReservePercent=20"
    "-XX:G1MixedGCCountTarget=3"
    "-XX:InitiatingHeapOccupancyPercent=20"
    "-XX:G1MixedGCLiveThresholdPercent=90"
    "-XX:G1SATBBufferEnqueueingThresholdPercent=30"
    "-XX:G1ConcMarkStepDurationMillis=5"
    "-XX:G1RSetUpdatingPauseTimePercent=0"
    "-XX:MaxGCPauseMillis=130"
    "-Djdk.graal.OptimizeLongJumps=true"
    "-Djdk.graal.TrivialInliningSize=16"
    "-Djdk.graal.MaximumInliningSize=320"
    "-Djdk.graal.MaximumEscapeAnalysisArrayLength=512"
    "-Djdk.graal.LoopRotation=true"
    "-Djdk.graal.TuneInlinerExploration=1"
    "-Djdk.graal.SpectrePHTBarriers=None"
    "-Djdk.graal.SpectrePHTIndexMasking=false"
    "-Djdk.nio.maxCachedBufferSize=262144"
    "-Dfml.readTimeout=180"
  ];
in
{
  options.${namespace}.services.minecraft.atm10-sky = {
    enable = mkEnableOption "Whether to run the ATM10 To the Sky Minecraft server.";
    port = mkOption {
      type = types.port;
      default = 25565;
    };
  };

  config = mkIf cfg.enable {
    services.minecraft-servers = {
      enable = true;
      eula = true;
      managementSystem = {
        tmux.enable = false;
        systemd-socket.enable = true;
      };
      servers.atm10-sky = {
        enable = true;
        autoStart = true;
        openFirewall = true;
        package = serverPackage;
        inherit jvmOpts;

        serverProperties = {
          server-port = cfg.port;
          motd = "Hosted by ElXreno";
          server-name = "ATM10 To the Sky";
          white-list = true;
          enforce-whitelist = true;
          allow-flight = true;
          sync-chunk-writes = false;
        };

        whitelist = {
          ElXreno = "40601523-a29f-414e-a942-799b839423ec";
          kontonkara = "723f7324-2389-4ef8-bf8e-85390ef0fab1";
        };

        symlinks = collectFilesAt modpack "mods";

        files =
          collectFilesAt modpack "config"
          // collectFilesAt modpack "defaultconfigs"
          // collectFilesAt modpack "kubejs"
          // {
            "config/bluemap/core.conf" = {
              format = pkgs.formats.json { };
              value = {
                accept-download = true;
                data = "bluemap";
                render-thread-count = 4;
                scan-for-mod-resources = true;
                metrics = true;
                log = {
                  file = "bluemap/logs/debug.log";
                  append = false;
                };
              };
            };
            "config/c2me.toml" = pkgs.writeText "c2me.toml" ''
              version = 3
              globalExecutorParallelism = 6

              [vanillaWorldGenOptimizations]
              useDensityFunctionCompiler = true

              [vanillaWorldGenOptimizations.nativeAcceleration]
              enabled = true
            '';
            "config/lootr-common.toml" = pkgs.writeText "lootr-common.toml" (
              builtins.readFile ../atm10/configs/lootr-common.toml
            );
            "config/ftbchunks-world.snbt" = pkgs.writeText "ftbchunks-world.snbt" (
              builtins.readFile ./configs/ftbchunks-world.snbt
            );
          };
      };
    };

    ${namespace} = {
      services.nginx = {
        enable = true;
        virtualHosts = {
          "map.elxreno.com" = {
            locations."/" = {
              proxyPass = "http://localhost:8100";
            };
          };
        };
      };

      system.impermanence.directories = [
        {
          directory = "/srv/minecraft";
          user = "minecraft";
          group = "minecraft";
          mode = "0750";
        }
      ];

      services.scx.layered.layers = {
        atm10-sky-server-thread = {
          priority = 10;
          matches = [
            [
              { CgroupContains = "minecraft-server-atm10-sky"; }
              { CommPrefix = "Server thread"; }
            ]
          ];
          kind.Grouped = {
            util_range = [
              0.4
              0.95
            ];
            cpus_range = [
              1
              2
            ];
            idle_confined = true;
            protected = true;
            preempt = true;
            preempt_first = true;
          };
        };
        atm10-sky-gc = {
          priority = 20;
          matches = [
            [
              { CgroupContains = "minecraft-server-atm10-sky"; }
              { CommPrefix = "GC Thread#"; }
            ]
            [
              { CgroupContains = "minecraft-server-atm10-sky"; }
              { CommPrefix = "G1 "; }
            ]
          ];
          kind.Grouped = {
            util_range = [
              0.05
              0.95
            ];
            cpus_range_frac = [
              0.0
              1.0
            ];
            preempt = true;
          };
        };
        atm10-sky-c2me = {
          priority = 30;
          matches = [
            [
              { CgroupContains = "minecraft-server-atm10-sky"; }
              { CommPrefix = "c2me-"; }
            ]
          ];
          kind.Grouped = {
            util_range = [
              0.0
              0.9
            ];
            cpus_range_frac = [
              0.3
              0.7
            ];
            idle_confined = true;
          };
        };
        atm10-sky-interactive = {
          priority = 25;
          matches = [
            [
              { CgroupContains = "minecraft-server-atm10-sky"; }
              { CommPrefix = "AE Crafting Cal"; }
            ]
          ];
          kind.Grouped = {
            util_range = [
              0.0
              0.95
            ];
            cpus_range_frac = [
              0.0
              0.5
            ];
            preempt = true;
          };
        };
        atm10-sky-bluemap = {
          priority = 100;
          matches = [
            [
              { CgroupContains = "minecraft-server-atm10-sky"; }
              { CommPrefix = "BlueMap-"; }
            ]
            [
              { CgroupContains = "minecraft-server-atm10-sky"; }
              { CommPrefix = "RenderManager-"; }
            ]
          ];
          kind.Open = {
            weight = 1;
          };
        };
        atm10-sky-jit = {
          priority = 110;
          matches = [
            [
              { CgroupContains = "minecraft-server-atm10-sky"; }
              { CommPrefix = "JVMCI-native"; }
            ]
            [
              { CgroupContains = "minecraft-server-atm10-sky"; }
              { CommPrefix = "C1 Compiler"; }
            ]
          ];
          kind.Open = {
            weight = 10;
          };
        };
      };
    };

    systemd.services.minecraft-server-atm10-sky.serviceConfig = {
      TimeoutStopSec = lib.mkForce "10min";
      MemorySwapMax = "0";
    };
  };
}
