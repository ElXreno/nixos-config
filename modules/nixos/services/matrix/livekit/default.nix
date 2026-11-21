{
  config,
  namespace,
  lib,
  pkgs,
  ...
}:

let
  inherit (lib) mkIf;
  cfg = config.${namespace}.services.matrix;
  ports = config.${namespace}.ports;

  baseDomain = cfg.serverName;
  livekitHost = "livekit.${baseDomain}";
  livekitServiceUrl = "https://${livekitHost}";
  keyFile = config.clan.core.vars.generators.matrix-livekit-key.files.key-file.path;

  rtcPorts = {
    from = 50000;
    to = 51000;
  };
in
{
  config = mkIf (cfg.enable && cfg.voip.enable) {
    clan.core.vars.generators.matrix-livekit-key = {
      files.key-file = {
        secret = true;
        restartUnits = [
          "livekit.service"
          "lk-jwt-service.service"
        ];
      };
      runtimeInputs = [ pkgs.openssl ];
      script = ''
        secret=$(openssl rand -hex 32 | tr -d '\n')
        printf 'lk-jwt-service: %s\n' "$secret" > "$out/key-file"
      '';
    };

    services.livekit = {
      enable = true;
      inherit keyFile;
      openFirewall = false;
      settings = {
        port = ports.livekit;
        rtc = {
          port_range_start = rtcPorts.from;
          port_range_end = rtcPorts.to;
          use_external_ip = true;
        };
      };
    };

    services.lk-jwt-service = {
      enable = true;
      inherit keyFile;
      livekitUrl = "wss://${livekitHost}";
      port = ports.lk-jwt;
    };

    systemd.services.lk-jwt-service.environment.LIVEKIT_FULL_ACCESS_HOMESERVERS = baseDomain;

    services.matrix-synapse.settings = {
      experimental_features.msc4143_enabled = true;
      matrix_rtc.transports = [
        {
          type = "livekit";
          livekit_service_url = livekitServiceUrl;
        }
      ];
    };

    ${namespace} = {
      system.networking.firewall.public.livekit.udpRanges = [ rtcPorts ];
      ports = {
        livekit = 7880;
        lk-jwt = 8083;
      };
      services.matrix.wellKnownClientExtra."org.matrix.msc4143.rtc_foci" = [
        {
          type = "livekit";
          livekit_service_url = livekitServiceUrl;
        }
      ];
    };

    services.caddy.virtualHosts."${livekitHost}".extraConfig = ''
      handle /sfu/* {
        reverse_proxy 127.0.0.1:${toString ports.lk-jwt}
      }
      handle {
        reverse_proxy 127.0.0.1:${toString ports.livekit}
      }
    '';
  };
}
