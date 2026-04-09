{
  config,
  namespace,
  lib,
  ...
}:

let
  inherit (lib)
    mkIf
    mkEnableOption
    ;
  cfg = config.${namespace}.services.matrix.lk-jwt-service;
  synapseCfg = config.${namespace}.services.matrix.synapse;
  livekitCfg = config.${namespace}.services.matrix.livekit;

  baseDomain = synapseCfg.serverName;
  livekitHost = "livekit.${baseDomain}";
  livekitServiceUrl = "https://${livekitHost}";
  jwtPort = 8083;
in
{
  options.${namespace}.services.matrix.lk-jwt-service = {
    enable = mkEnableOption "Whether to manage lk-jwt-service for Element Call.";
  };

  config = mkIf cfg.enable {
    assertions = [
      {
        assertion = livekitCfg.enable;
        message = "${namespace}.services.matrix.lk-jwt-service requires matrix.livekit to be enabled.";
      }
    ];

    services.lk-jwt-service = {
      enable = true;
      keyFile = config.clan.core.vars.generators.matrix-livekit-key.files.key-file.path;
      livekitUrl = "wss://${livekitHost}";
      port = jwtPort;
    };

    systemd.services.lk-jwt-service.environment = {
      LIVEKIT_FULL_ACCESS_HOMESERVERS = baseDomain;
    };

    services.matrix-synapse.settings = {
      experimental_features.msc4143_enabled = true;
      matrix_rtc.transports = [
        {
          type = "livekit";
          livekit_service_url = livekitServiceUrl;
        }
      ];
    };

    ${namespace}.services.matrix.synapse.wellKnownClientExtra = {
      "org.matrix.msc4143.rtc_foci" = [
        {
          type = "livekit";
          livekit_service_url = livekitServiceUrl;
        }
      ];
    };

    services.nginx.virtualHosts."${livekitHost}".locations."/sfu/" = {
      proxyPass = "http://127.0.0.1:${toString jwtPort}/sfu/";
    };
  };
}
