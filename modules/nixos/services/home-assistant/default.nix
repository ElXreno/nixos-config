{
  config,
  namespace,
  lib,
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
    ${namespace}.services = {
      zigbee2mqtt.enable = cfg.zigbee2mqtt.enable;
      postgresql.enable = true;
    };

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
      ];
      extraPackages =
        python3Packages: with python3Packages; [
          psycopg2
          zlib-ng
        ];

      config = {
        # https://www.home-assistant.io/integrations/default_config/
        default_config = { };
        homeassistant = { };

        recorder.db_url = "postgresql://@/hass";
      };
    };
  };
}
