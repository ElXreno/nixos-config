{
  config,
  namespace,
  lib,
  inputs,
  ...
}:
let
  inherit (lib)
    mkIf
    mkEnableOption
    ;
  cfg = config.${namespace}.system.networking.wireless;
in
{
  options.${namespace}.system.networking.wireless = {
    enable = mkEnableOption "Whether or not to manage wireless networking.";
  };

  config = mkIf cfg.enable {
    sops = {
      secrets."ap/Home".sopsFile = "${inputs.self}/secrets/wireless.yaml";
      secrets."ap/Homelander".sopsFile = "${inputs.self}/secrets/wireless.yaml";

      templates."wpa_supplicant.conf" = {
        restartUnits = [ "wpa_supplicant.service" ];
        owner = "wpa_supplicant";
        content = ''
          network={
            ssid="Home"
            key_mgmt=WPA-PSK SAE FT-PSK FT-SAE
            psk=${config.sops.placeholder."ap/Home"}
          }

          network={
            ssid="Homelander"
            key_mgmt=SAE FT-SAE
            sae_password="${config.sops.placeholder."ap/Homelander"}"
            ieee80211w=2
          }
        '';
      };
    };

    networking.wireless = {
      enable = true;
      fallbackToWPA2 = false;
      extraConfigFiles = [ config.sops.templates."wpa_supplicant.conf".path ];
    };

    systemd.services.wpa_supplicant.serviceConfig = {
      BindReadOnlyPaths = [ config.sops.templates."wpa_supplicant.conf".path ];
    };
  };
}
