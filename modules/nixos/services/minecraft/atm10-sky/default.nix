{
  config,
  options,
  inputs,
  namespace,
  lib,
  pkgs,
  ...
}:
let
  inherit (lib)
    mkIf
    mkMerge
    mkEnableOption
    mkOption
    types
    ;
  inherit (inputs.nix-minecraft.lib) collectFilesAt;
  cfg = config.${namespace}.services.minecraft.atm10-sky;

  # The world must stop changing while kopia reads it, and autosave must come
  # back even if the snapshot dies.
  stdin = config.services.minecraft-servers.managementSystem.systemd-socket.stdinSocket.path "atm10-sky";
  systemctl = "${config.systemd.package}/bin/systemctl";
  journalctl = "${config.systemd.package}/bin/journalctl";
  saveOff = ''
    if ${systemctl} is-active --quiet minecraft-server-atm10-sky; then
      since=$(date '+%Y-%m-%d %H:%M:%S')
      echo 'save-off' > ${stdin}
      echo 'save-all flush' > ${stdin}
      timeout 600 grep -qm1 'Saved the game' \
        < <(${journalctl} -u minecraft-server-atm10-sky --since="$since" -f -o cat)
    fi
  '';
  saveOn = ''
    if ${systemctl} is-active --quiet minecraft-server-atm10-sky; then
      echo 'save-on' > ${stdin}
    fi
  '';

  modpack = pkgs.${namespace}.atm10-sky-modpack;

  serverPackage = modpack.serverPackage.override {
    jre_headless = pkgs.graalvmPackages.graalvm-oracle;
  };

  jvmOpts = lib.concatStringsSep " " [
    "-Xmx10G"
    "-Xms4G"
    "-XX:+UnlockExperimentalVMOptions"
    "-XX:+UnlockDiagnosticVMOptions"
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
    "-XX:+UseZGC"
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
    enable = mkEnableOption "running the ATM10 To the Sky Minecraft server";
    retain = mkEnableOption "backing up its data while the service is off" // {
      default = options.${namespace}.services.minecraft.atm10-sky.enable.highestPrio < 1500;
      defaultText = "Whether this host sets `enable` at all, even to false.";
    };
    port = mkOption {
      type = types.port;
      default = 25565;
    };
  };

  config = mkMerge [
    (mkIf (cfg.enable || cfg.retain) {
      ${namespace} = {
        services.kopia.backup = [
          {
            path = "/srv/minecraft/atm10-sky";
            ignore = [ "/simplebackups" ];
            preCommand = saveOff;
            postCommand = saveOn;
          }
        ];

        system.impermanence.directories = [
          {
            directory = "/srv/minecraft";
            user = "minecraft";
            group = "minecraft";
            mode = "0750";
          }
        ];
      };
    })
    (mkIf cfg.enable {
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
              "config/fml.toml" = pkgs.writeText "fml.toml" (builtins.readFile ./configs/fml.toml);
            };
        };
      };

      ${namespace} = {
        ports = {
          atm10-sky = cfg.port;
          atm10-sky-map = 8100;
        };

        system.networking = {
          dns.records."map.elxreno.com".proxied = true;
          firewall.public.minecraft = {
            upstream = "modules/minecraft-servers.nix";
            tcp = [ cfg.port ];
          };
        };

        services.caddy.enable = true;

      };

      services.caddy.virtualHosts."map.elxreno.com".extraConfig = ''
        reverse_proxy localhost:${toString config.${namespace}.ports.atm10-sky-map}
      '';

      systemd.services.minecraft-server-atm10-sky.serviceConfig = {
        TimeoutStopSec = lib.mkForce "10min";
      };
    })
  ];
}
