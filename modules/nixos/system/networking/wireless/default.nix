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
    mkOption
    types
    ;
  cfg = config.${namespace}.system.networking.wireless;

  promptName = ssid: "${lib.toLower ssid}-password";

  networkBlock =
    ssid: network:
    let
      password = ''"$(cat "$prompts/${promptName ssid}")"'';
    in
    if network.saeOnly then
      ''
        network={
          ssid="${ssid}"
          key_mgmt=SAE FT-SAE
          sae_password=${password}
          ieee80211w=2
        }
      ''
    else
      ''
        network={
          ssid="${ssid}"
          key_mgmt=WPA-PSK SAE FT-PSK FT-SAE
          psk=$(wpa_passphrase "${ssid}" ${password} | grep -oE 'psk=[0-9a-f]{64}' | cut -d= -f2)
        }
      '';
in
{
  options.${namespace}.system.networking.wireless = {
    enable = mkEnableOption "managing wireless networking";

    networks = mkOption {
      type = types.attrsOf (
        types.submodule {
          options.saeOnly = mkEnableOption "WPA3-only authentication with management frame protection";
        }
      );
      default = { };
      example = {
        Home = { };
        Office.saeOnly = true;
      };
      description = "Access points keyed by SSID; each one prompts for its password once.";
    };
  };

  config = mkIf cfg.enable {
    clan.core.vars.generators.wireless-aps = {
      prompts = lib.mapAttrs' (
        ssid: _:
        lib.nameValuePair (promptName ssid) {
          description = "${ssid} AP Password";
          type = "hidden";
        }
      ) cfg.networks;

      files."wpa_supplicant.conf" = {
        secret = true;
        owner = "wpa_supplicant";
      };

      script = ''
        cat > "$out/wpa_supplicant.conf" << EOF
        ${lib.concatStringsSep "\n" (lib.mapAttrsToList networkBlock cfg.networks)}EOF
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
  };
}
