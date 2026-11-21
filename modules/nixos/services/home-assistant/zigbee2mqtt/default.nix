{
  config,
  namespace,
  lib,
  pkgs,
  ...
}:

let
  inherit (lib) mkIf;
  cfg = config.${namespace}.services.home-assistant;
  ports = config.${namespace}.ports;
in
{
  config = mkIf cfg.enable {
    ${namespace} = {
      ports = {
        zigbee2mqtt = 8083;
        mosquitto = 1883;
      };
      tailscale.serve."svc:z2m" = {
        comment = "Zigbee2MQTT frontend";
        tags = [ "tag:hass" ];
        endpoints."tcp:443" = "http://localhost:${toString ports.zigbee2mqtt}";
      };
    };

    services.mosquitto.enable = true;

    services.zigbee2mqtt = {
      enable = true;
      settings = {
        homeassistant.enabled = true;
        devices = {
          "0xa4c138b5be94eddd" = {
            friendly_name = "plug_ceiling_lamp";
          };
        };
        serial = {
          port = "/dev/serial/by-id/usb-Itead_Sonoff_Zigbee_3.0_USB_Dongle_Plus_V2_1e7774bc0674ef11828de21e313510fd-if00-port0";
          adapter = "ember";
        };
        frontend = {
          enabled = true;
          port = ports.zigbee2mqtt;
        };
        availability.enabled = true;
        ota.zigbee_ota_override_index_location = pkgs.writeText "ota-config.json" (
          builtins.toJSON [
            {
              url = pkgs.fetchurl {
                url = "https://github.com/pvvx/ATC_MiThermometer/raw/refs/heads/master/zigbee_ota/1141-020a-99993001-ATC_v56.zigbee";
                hash = "sha256-mYK+8ti5b23NIE4/myCeuTocp9kcF/Wwbo98NOgRlik=";
              };
              modelId = "LYWSD03MMC-z";
              force = true;
            }
          ]
        );
      };
    };
  };
}
