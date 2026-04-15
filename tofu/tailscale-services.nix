{
  config,
  pkgs,
  lib,
  ...
}:
let
  mkClanSecret = name: {
    program = [
      (lib.getExe (
        pkgs.writeShellApplication {
          inherit name;
          runtimeInputs = [ pkgs.jq ];
          text = ''
            jq -n --arg secret "$(clan secrets get tf-${name})" '{"secret":$secret}'
          '';
        }
      ))
    ];
  };
in
{
  variable.passphrase = { };

  terraform.required_providers.tailscale.source = "tailscale/tailscale";
  terraform.required_providers.external.source = "hashicorp/external";

  data.external.tailscale-oauth-client-id = mkClanSecret "tailscale-oauth-client-id";
  data.external.tailscale-oauth-client-secret = mkClanSecret "tailscale-oauth-client-secret";

  provider.tailscale = {
    oauth_client_id = config.data.external.tailscale-oauth-client-id "result.secret";
    oauth_client_secret = config.data.external.tailscale-oauth-client-secret "result.secret";
  };

  resource.tailscale_service = {
    radarr = {
      name = "svc:radarr";
      comment = "Radarr movie manager";
      ports = [ "tcp:443" ];
      tags = [ "tag:nixflix" ];
    };

    sonarr = {
      name = "svc:sonarr";
      comment = "Sonarr TV show manager";
      ports = [ "tcp:443" ];
      tags = [ "tag:nixflix" ];
    };

    qbittorrent = {
      name = "svc:qbittorrent";
      comment = "qBittorrent download client";
      ports = [ "tcp:443" ];
      tags = [ "tag:nixflix" ];
    };

    jellyfin = {
      name = "svc:jellyfin";
      comment = "Jellyfin media server";
      ports = [ "tcp:443" ];
      tags = [ "tag:nixflix" ];
    };

    prowlarr = {
      name = "svc:prowlarr";
      comment = "Prowlarr indexer manager";
      ports = [ "tcp:443" ];
      tags = [ "tag:nixflix" ];
    };

    hass = {
      name = "svc:hass";
      comment = "Home Assistant";
      ports = [ "tcp:443" ];
      tags = [ "tag:hass" ];
    };

    z2m = {
      name = "svc:z2m";
      comment = "Zigbee2MQTT frontend";
      ports = [ "tcp:443" ];
      tags = [ "tag:hass" ];
    };
  };

  output = {
    radarr_addrs.value = "\${tailscale_service.radarr.addrs}";
    sonarr_addrs.value = "\${tailscale_service.sonarr.addrs}";
    qbittorrent_addrs.value = "\${tailscale_service.qbittorrent.addrs}";
    jellyfin_addrs.value = "\${tailscale_service.jellyfin.addrs}";
    prowlarr_addrs.value = "\${tailscale_service.prowlarr.addrs}";
    hass_addrs.value = "\${tailscale_service.hass.addrs}";
    z2m_addrs.value = "\${tailscale_service.z2m.addrs}";
  };
}
