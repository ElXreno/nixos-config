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
  cfg = config.${namespace}.services.matrix.livekit;
  synapseCfg = config.${namespace}.services.matrix.synapse;

  baseDomain = synapseCfg.serverName;
  livekitHost = "livekit.${baseDomain}";
in
{
  options.${namespace}.services.matrix.livekit = {
    enable = mkEnableOption "Whether to manage LiveKit SFU for Element Call.";
  };

  config = mkIf cfg.enable {
    assertions = [
      {
        assertion = synapseCfg.enable;
        message = "${namespace}.services.matrix.livekit requires matrix.synapse to be enabled.";
      }
    ];

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
      keyFile = config.clan.core.vars.generators.matrix-livekit-key.files.key-file.path;
      openFirewall = false;
      settings = {
        port = 7880;
        rtc = {
          port_range_start = 50000;
          port_range_end = 51000;
          use_external_ip = true;
        };
      };
    };

    networking.firewall.allowedUDPPortRanges = [
      {
        from = 50000;
        to = 51000;
      }
    ];

    ${namespace}.services.nginx.virtualHosts."${livekitHost}" = {
      locations."/" = {
        proxyPass = "http://127.0.0.1:7880";
        proxyWebsockets = true;
        extraConfig = ''
          proxy_buffering off;
          proxy_read_timeout 86400s;
          proxy_send_timeout 86400s;
        '';
      };
    };
  };
}
