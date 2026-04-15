{
  config,
  namespace,
  lib,
  pkgs,
  ...
}:

let
  inherit (lib) mkIf mkEnableOption;
  cfg = config.${namespace}.services.home-assistant;
in
{
  options.${namespace}.services.home-assistant = {
    enable = mkEnableOption "Whether or not to manage home-assistant.";
    zigbee2mqtt.enable = mkEnableOption "Whether to enable zigbee2mqtt." // {
      default = true;
    };
  };

  config = mkIf cfg.enable {
    ${namespace} = {
      services = {
        zigbee2mqtt.enable = cfg.zigbee2mqtt.enable;
        postgresql.enable = true;
      };
      tailscale.serve."svc:hass".endpoints."tcp:443" = "http://localhost:8123";
    };

    networking.firewall.allowedTCPPorts = [ 8123 ];

    systemd.tmpfiles.rules = [
      "f /var/lib/hass/automations.yaml 0644 hass hass - []"
      "f /var/lib/hass/scripts.yaml 0644 hass hass - {}"
      "f /var/lib/hass/scenes.yaml 0644 hass hass - []"
      "f /var/lib/hass/templates.yaml 0644 hass hass - []"
    ];

    # TODO: move to ${namespace}.services.postgresql
    services.postgresql = {
      ensureDatabases = [ "hass" ];
      ensureUsers = [
        {
          name = "hass";
          ensureDBOwnership = true;
        }
      ];
    };

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
        "xiaomi_ble"
        "keenetic_ndms2"
        "ibeacon"

        "syncthing"
        "jellyfin"
        "upnp"
        "seventeentrack"
      ];
      extraPackages =
        python3Packages: with python3Packages; [
          psycopg2
          zlib-ng
        ];

      customComponents = with pkgs.${namespace}; [
        hass-dreame-vacuum
      ];

      config = {
        # https://www.home-assistant.io/integrations/default_config/
        default_config = { };
        homeassistant = { };

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
  };
}
