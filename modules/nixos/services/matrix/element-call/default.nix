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
  cfg = config.${namespace}.services.matrix.element-call;
  synapseCfg = config.${namespace}.services.matrix.synapse;

  baseDomain = synapseCfg.serverName;
  matrixHost = "matrix.${baseDomain}";
  livekitHost = "livekit.${baseDomain}";
  callHost = "call.${baseDomain}";

  jsonFormat = pkgs.formats.json { };

  elementCallConfig = jsonFormat.generate "element-call-config.json" {
    default_server_config = {
      "m.homeserver" = {
        base_url = "https://${matrixHost}";
        server_name = baseDomain;
      };
    };
    livekit = {
      livekit_service_url = "https://${livekitHost}";
    };
  };

  elementCallDist = pkgs.runCommand "element-call-dist" { } ''
    mkdir -p $out
    cp -r ${pkgs.element-call}/. $out/
    chmod u+w $out
    install -m 0644 ${elementCallConfig} $out/config.json
  '';
in
{
  options.${namespace}.services.matrix.element-call = {
    enable = mkEnableOption "Whether to manage Element Call frontend.";
  };

  config = mkIf cfg.enable {
    assertions = [
      {
        assertion = synapseCfg.enable;
        message = "${namespace}.services.matrix.element-call requires matrix.synapse to be enabled.";
      }
    ];

    ${namespace}.services.nginx.virtualHosts."${callHost}" = {
      root = "${elementCallDist}";
      locations."/" = {
        tryFiles = "$uri $uri/ /index.html";
      };
      locations."= /config.json" = {
        extraConfig = ''
          expires -1;
        '';
      };
      locations."/assets/" = {
        extraConfig = ''
          expires 1y;
        '';
      };
    };
  };
}
