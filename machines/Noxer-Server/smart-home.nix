{ config, pkgs, lib, ... }:
with lib;
let
  cfg = config.services.smart-home-server;
in
{
  options.services.smart-home-server = {
    enable = mkEnableOption "smart-home-server";

    package = mkOption {
      type = types.package;
      default = pkgs.smart-home-server;
      defaultText = literalExpression "pkgs.smart-home-server";
      description = "Which smart-home-server package to use.";
    };

    psqlPackage = mkOption {
      type = types.package;
      default = pkgs.postgresql_11;
      defaultText = literalExpression "pkgs.postgresql_11";
      description = "Which postgresql package to use.";
    };

    nginxVirtualHost = mkOption {
      type = types.str;
      default = "api.example.com";
      description = "Which VirtualHost should be used for `addToNginx` parameter.";
    };

    addToNginx = mkOption {
      type = types.bool;
      default = false;
      description = "Whether smart-home-server should be added to nginx configuration.";
    };
  };

  config = mkIf cfg.enable {
    systemd.packages = [ cfg.package ];

    systemd.services.smart-home-server = {
      path = [ cfg.package ];
      serviceConfig = {
        ExecStart = [ "${cfg.package}/bin/ServerApp" ];
      };
      wantedBy = [ "default.target" ];
    };

    services.nginx.virtualHosts.${cfg.nginxVirtualHost} = mkIf cfg.addToNginx {
      locations."/" = { proxyPass = "http://localhost:5152"; };
    };

    services.postgresql = {
      enable = true;
      package = cfg.psqlPackage;
    };
  };
}
