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
  cfg = config.${namespace}.system.virtualisation.containers.minecraft.atm10;
in
{
  options.${namespace}.system.virtualisation.containers.minecraft.atm10 = {
    enable = mkEnableOption "Whether or not to manage podman.";
    image = mkOption {
      type = types.str;
      default = "itzg/minecraft-server:2026.1.0-java25-graalvm";
    };
    dataDir = mkOption {
      type = types.str;
      default = "/var/lib/minecraft/atm10";
    };
    port = mkOption {
      type = types.port;
      default = 25565;
    };
  };

  config = mkIf cfg.enable {
    sops.secrets."curseforge" = { };

    systemd.tmpfiles.rules = [
      "d ${cfg.dataDir} 0770 root users -"
    ];

    ${namespace}.services.nginx = {
      enable = true;
      virtualHosts = {
        "map.elxreno.com" = {
          locations."/" = {
            proxyPass = "http://localhost:8100";
          };
        };
      };
    };

    networking.firewall.allowedUDPPorts = [
      24454 # Simple Voice Chat
    ];

    boot.kernel.sysctl = {
      "vm.nr_hugepages" = 7680; # For Java
    };

    virtualisation.oci-containers.containers = {
      minecraft-atm10 = {
        autoStart = true;
        inherit (cfg) image;
        volumes = [
          "/etc/localtime:/etc/localtime:ro"

          "${cfg.dataDir}:/data"
        ];
        ports = [
          "${toString cfg.port}:${toString cfg.port}/tcp"
          "${toString cfg.port}:${toString cfg.port}/udp"
          "24454:24454/udp" # Simple Voice Chat
          "127.0.0.1:8100:8100/tcp" # bluemap
        ];

        environmentFiles = [ config.sops.secrets."curseforge".path ];
        environment = {
          DEBUG = "true";
          DEBUG_EXEC = "true";

          EULA = "true";
          SERVER_PORT = "${toString cfg.port}";

          STOP_DURATION = "240";

          TYPE = "AUTO_CURSEFORGE";
          CF_SLUG = "all-the-mods-10";
          CF_FILE_ID = "7558573";
          CF_OVERRIDES_EXCLUSIONS = "shaderpacks/**";

          INIT_MEMORY = "12G";
          MAX_MEMORY = "12G";

          ### VIBE ARGS START ENTRY ###
          JVM_OPTS = ''
            -XX:+UnlockExperimentalVMOptions
            -XX:+UnlockDiagnosticVMOptions

            -XX:+AlwaysPreTouch
            -XX:+UseLargePages
            -XX:LargePageSizeInBytes=2m

            -XX:+DisableExplicitGC
            -XX:+PerfDisableSharedMem
            -XX:+UseStringDeduplication
            -XX:+UseCriticalJavaThreadPriority
            -XX:+UseFastUnorderedTimeStamps
            -XX:+AlwaysActAsServerClassMachine
            -XX:AllocatePrefetchStyle=3

            -XX:-DontCompileHugeMethods
            -XX:MaxNodeLimit=240000
            -XX:NodeLimitFudgeFactor=8000
            -XX:ReservedCodeCacheSize=400M
            -XX:NonNMethodCodeHeapSize=12M
            -XX:ProfiledCodeHeapSize=194M
            -XX:NonProfiledCodeHeapSize=194M
            -XX:NmethodSweepActivity=1
            -XX:+UseVectorStubs

            -XX:G1NewSizePercent=40
            -XX:G1MaxNewSizePercent=50
            -XX:G1HeapRegionSize=16M
            -XX:G1ReservePercent=15
            -XX:G1MixedGCCountTarget=3
            -XX:InitiatingHeapOccupancyPercent=20
            -XX:G1MixedGCLiveThresholdPercent=90
            -XX:SurvivorRatio=32
            -XX:MaxTenuringThreshold=1
            -XX:G1SATBBufferEnqueueingThresholdPercent=30
            -XX:G1ConcMarkStepDurationMillis=5
            -XX:G1RSetUpdatingPauseTimePercent=0
            -XX:MaxGCPauseMillis=130

            -Djdk.graal.TuneInlinerExploration=1
            -Djdk.graal.OptimizeLongJumps=true
            -Djdk.graal.OptMethodDuplication=true
            -Djdk.graal.TrivialInliningSize=25
            -Djdk.graal.MaximumInliningSize=800
            -Djdk.graal.MaximumEscapeAnalysisArrayLength=512
            -Djdk.graal.DuplicationBudgetFactor=0.75
            -Djdk.graal.MaxDuplicationFactor=4.0
            -Djdk.graal.TypicalGraphSize=8000
            -Djdk.graal.SpectrePHTBarriers=None
            -Djdk.graal.SpectrePHTIndexMasking=false

            -XX:+AutoCreateSharedArchive
            -XX:SharedArchiveFile=atm10_cds.jsa

            -Djdk.nio.maxCachedBufferSize=262144
            -Dfml.readTimeout=180
          '';
          ### VIBE ARGS END ENTRY ###

          SERVER_NAME = "ATM10";
          MOTD = "Hosted by ElXreno";
          ICON = "https://i.postimg.cc/vZgh2sJQ/minecraft.jpg";

          ENABLE_WHITELIST = "true";
          WHITELIST = "ElXreno,kontonkara,tsalkenov,SaymoonAmogus,G_R_O_M_I_L_A";

          ALLOW_FLIGHT = "true";

          MODRINTH_PROJECTS = ''
            packet-fixer:2C41Q8WX
            bluemap:8iJcPOHJ
          '';
        };
        extraOptions = [
          "--hostname=minecraft-atm10"
          "--pull=newer"
          "--stop-timeout=300"
        ];

        podman.sdnotify = "healthy"; # avoid nasty errors with deploy-rs about healthcheck
      };
    };
  };
}
