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

  baseDomain = cfg.serverName;
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
  config = mkIf (cfg.enable && cfg.voip.enable) {
    services.caddy.virtualHosts."${callHost}".extraConfig = ''
      root * ${elementCallDist}
      @config path /config.json
      header @config Cache-Control no-cache
      @assets path /assets/*
      header @assets Cache-Control "public, max-age=31536000"
      try_files {path} {path}/ /index.html
      file_server
    '';
  };
}
