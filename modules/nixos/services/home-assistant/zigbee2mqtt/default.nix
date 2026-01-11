{
  config,
  namespace,
  lib,
  ...
}:

let
  inherit (lib) mkIf mkEnableOption;
  cfg = config.${namespace}.services.zigbee2mqtt;
in
{
  options.${namespace}.services.zigbee2mqtt = {
    enable = mkEnableOption "Whether or not to manage zigbee2mqtt.";
  };

  config = mkIf cfg.enable {
    ${namespace}.services.mosquitto.enable = true;

    services.zigbee2mqtt = {
      enable = true;
      settings = {
        advanced = {
          homeassistant_legacy_entity_attributes = false;
          homeassistant_legacy_triggers = false;
          legacy_api = false;
          legacy_availability_payload = false;
        };
        device_option = {
          legacy = false;
        };
        homeassistant.enabled = config.${namespace}.services.home-assistant.enable;
        permit_join = true;
        serial = {
          port = "/dev/serial/by-id/usb-Itead_Sonoff_Zigbee_3.0_USB_Dongle_Plus_V2_1e7774bc0674ef11828de21e313510fd-if00-port0";
          adapter = "ember";
        };
        frontend = {
          enabled = true;
          port = 8083;
        };
        availability.enabled = true;
        # ota.zigbee_ota_override_index_location = pkgs.writeText "ota-config.json" (
        #   builtins.toJSON [
        #     {
        #       url = (
        #         pkgs.fetchurl {
        #           url = "https://github.com/devbis/z03mmc/raw/refs/heads/master/assets/db15-0203-99993001-ATC_v46.zigbee";
        #           hash = "sha256-4t7WxFa+/iyS/P3+mfbhrFjW6RAbZ+cf0ViO237pFyc=";
        #         }
        #       );
        #       force = true;
        #     }
        #   ]
        # );
      };
    };
  };
}
