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
      default = "itzg/minecraft-server:java25-graalvm";
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

    virtualisation.oci-containers.containers = {
      minecraft-atm10 = {
        autoStart = true;
        image = cfg.image;
        volumes = [
          "/etc/localtime:/etc/localtime:ro"

          "${cfg.dataDir}:/data"
        ];
        ports = [
          "${toString cfg.port}:${toString cfg.port}/tcp"
          "${toString cfg.port}:${toString cfg.port}/udp"
          "24454:24454/udp" # Simple Voice Chat
        ];

        environmentFiles = [ config.sops.secrets."curseforge".path ];
        environment = {
          DEBUG = "true";
          DEBUG_EXEC = "true";

          EULA = "true";
          SERVER_PORT = "${toString cfg.port}";

          TYPE = "AUTO_CURSEFORGE";
          CF_SLUG = "all-the-mods-10";
          CF_FILE_ID = "7271400";
          CF_OVERRIDES_EXCLUSIONS = "shaderpacks/**";

          INIT_MEMORY = "12G";
          MAX_MEMORY = "12G";
          USE_MEOWICE_FLAGS = "true";
          USE_MEOWICE_GRAALVM_FLAGS = "true";
          JVM_OPTS = "-Dfml.readTimeout=180 -XX:+AutoCreateSharedArchive -XX:SharedArchiveFile=atm10_cds.jsa";

          SERVER_NAME = "ATM10";
          MOTD = "Hosted by ElXreno";
          ICON = "https://i.postimg.cc/vZgh2sJQ/minecraft.jpg";

          ENABLE_WHITELIST = "true";
          WHITELIST = "ElXreno,kontonkara,tsalkenov";

          MODRINTH_PROJECTS = ''
            packet-fixer:2C41Q8WX
          '';
        };
        extraOptions = [
          "--hostname=minecraft-atm10"
          "--pull=newer"
        ];

        podman.sdnotify = "healthy"; # avoid nasty errors with deploy-rs about healthcheck
      };
    };
  };
}
