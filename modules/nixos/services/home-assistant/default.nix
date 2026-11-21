{
  config,
  options,
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
    ;
  cfg = config.${namespace}.services.home-assistant;
  ports = config.${namespace}.ports;
in
{
  options.${namespace}.services.home-assistant = {
    enable = mkEnableOption "managing home-assistant";
    retain = mkEnableOption "backing up its data while the service is off" // {
      default = options.${namespace}.services.home-assistant.enable.highestPrio < 1500;
      defaultText = "Whether this host sets `enable` at all, even to false.";
    };
  };

  config = mkMerge [
    (mkIf (cfg.enable || cfg.retain) {
      ${namespace}.services = {
        postgresql = {
          enable = true;
          databases.hass.backup = true;
        };
        kopia.backup = [
          {
            path = "/var/lib/hass";
            ignore = [
              "/.cache"
              "/backups"
              "/backup-migration-*"
              "/home-assistant.log*"
            ];
          }
        ];
      };
    })
    (mkIf cfg.enable {
      ${namespace} = {
        ports.home-assistant = 8123;
        system.networking.firewall.public.home-assistant.tcp = [ ports.home-assistant ];
        services.cloudflared.ingress."hass.elxreno.com" =
          "http://localhost:${toString ports.home-assistant}";
        tailscale.serve."svc:hass" = {
          comment = "Home Assistant";
          tags = [ "tag:hass" ];
          endpoints."tcp:443" = "http://localhost:${toString ports.home-assistant}";
        };
      };

      systemd.tmpfiles.rules = [
        "f /var/lib/hass/automations.yaml 0644 hass hass - []"
        "f /var/lib/hass/scripts.yaml 0644 hass hass - {}"
        "f /var/lib/hass/scenes.yaml 0644 hass hass - []"
        "f /var/lib/hass/templates.yaml 0644 hass hass - []"
      ];

      services.home-assistant = {
        enable = true;

        extraComponents = [
          # Temporary
          "analytics"
          "google_translate"
          "met"
          "radio_browser"
          "shopping_list"

          # Persistent
          "mqtt"
          "zha"
          "isal"
          "bthome"
          "kegtron"
          "xiaomi_ble"
          "keenetic_ndms2"
          "ibeacon"
          "ffmpeg"
          "homekit"

          "jellyfin"
          "upnp"
          "seventeentrack"
        ];
        extraPackages =
          python3Packages: with python3Packages; [
            psycopg2
            zlib-ng
          ];

        customComponents =
          (with pkgs.${namespace}; [
            hass-dreame-vacuum
          ])
          ++ (with pkgs.home-assistant-custom-components; [
            xiaomi_miot
          ]);

        customLovelaceModules = with pkgs.${namespace}; [
          hass-lovelace-xiaomi-vacuum-map-card
        ];

        config = {
          # https://www.home-assistant.io/integrations/default_config/
          default_config = { };
          homeassistant.external_url = "https://hass.elxreno.com";

          automation = "!include automations.yaml";
          script = "!include scripts.yaml";
          scene = "!include scenes.yaml";
          template = "!include templates.yaml";

          http = {
            use_x_forwarded_for = true;
            trusted_proxies = [
              "127.0.0.1"
              "::1"
            ];
          };

          recorder.db_url = "postgresql://@/hass";
        };
      };
    })
  ];
}
