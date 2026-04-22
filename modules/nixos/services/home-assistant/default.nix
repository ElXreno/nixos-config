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
    concatMapStringsSep
    splitString
    filter
    ;
  cfg = config.${namespace}.services.home-assistant;

  cloudflareIpRanges = filter (s: s != "") (
    splitString "\n" (
      builtins.readFile (
        pkgs.fetchurl {
          url = "https://www.cloudflare.com/ips-v4";
          hash = "sha256-8Cxtg7wBqwroV3Fg4DbXAMdFU1m84FTfiE5dfZ5Onns=";
        }
      )
    )
  );

  realIpFromCloudflare = concatMapStringsSep "\n" (
    cidr: "set_real_ip_from ${cidr};"
  ) cloudflareIpRanges;
  cfAllowedGeoEntries = concatMapStringsSep "\n" (cidr: "${cidr} 1;") cloudflareIpRanges;
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
        nginx = {
          enable = true;
          virtualHosts."hass.elxreno.com" = {
            acmeRoot = null;
            extraConfig = ''
              ${realIpFromCloudflare}
              real_ip_header CF-Connecting-IP;
              if ($cf_allowed = 0) { return 403; }
            '';
            locations."/" = {
              proxyPass = "http://127.0.0.1:8123";
              proxyWebsockets = true;
            };
          };
        };
      };
      tailscale.serve."svc:hass".endpoints."tcp:443" = "http://localhost:8123";
    };

    networking.firewall.allowedTCPPorts = [ 8123 ];

    services.nginx.appendHttpConfig = ''
      geo $realip_remote_addr $cf_allowed {
        default 0;
        127.0.0.1 1;
        ${cfAllowedGeoEntries}
      }
    '';

    security.acme.certs."hass.elxreno.com" = {
      dnsProvider = "cloudflare";
      extraLegoFlags = [ "--dns.propagation-wait=60s" ];
      credentialFiles.CLOUDFLARE_DNS_API_TOKEN_FILE =
        config.clan.core.vars.generators.acme-cloudflare.files.token.path;
    };

    clan.core.vars.generators.acme-cloudflare = {
      prompts.token = {
        description = "Cloudflare API token (Zone:Zone:Read + Zone:DNS:Edit)";
        type = "hidden";
        persist = true;
      };
    };

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
        "ffmpeg"
        "homekit"

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
  };
}
