{
  config,
  namespace,
  lib,
  pkgs,
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
    disablePowerSave = mkEnableOption "Whether to disable Wi-Fi power save." // {
      default = true;
    };
  };

  config = mkIf cfg.enable {
    clan.core.vars.generators.wireless-aps = {
      prompts = {
        home-password = {
          description = "Home AP Password";
          type = "hidden";
        };
        homelander-password = {
          description = "Homelander AP Password";
          type = "hidden";
        };
      };

      files."wpa_supplicant.conf" = {
        secret = true;
        owner = "wpa_supplicant";
      };

      script = ''
        cat > "$out/wpa_supplicant.conf" << EOF
        network={
          ssid="Home"
          key_mgmt=WPA-PSK SAE FT-PSK FT-SAE
          psk=$(wpa_passphrase Home $(cat $prompts/home-password) | grep -oE 'psk=[0-9a-f]{64}' | cut -d= -f2)
        }

        network={
          ssid="Homelander"
          key_mgmt=SAE FT-SAE
          sae_password="$(cat "$prompts/homelander-password")"
          ieee80211w=2
        }
        EOF
      '';
      runtimeInputs = with pkgs; [
        wpa_supplicant
        gnugrep
      ];
    };

    networking.wireless = {
      enable = true;
      fallbackToWPA2 = false;
      extraConfigFiles = [
        config.clan.core.vars.generators.wireless-aps.files."wpa_supplicant.conf".path
      ];
    };

    systemd.services.wpa_supplicant.serviceConfig = {
      BindReadOnlyPaths = [
        config.clan.core.vars.generators.wireless-aps.files."wpa_supplicant.conf".path
      ];
    };

    boot.extraModprobeConfig = mkIf cfg.disablePowerSave ''
      options iwlmvm power_scheme=1
    '';
  };
}
