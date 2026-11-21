{
  config,
  namespace,
  lib,
  ...
}:
let
  inherit (lib)
    hasSuffix
    mkBefore
    mkDefault
    mkEnableOption
    mkIf
    mkMerge
    mkOption
    replaceStrings
    types
    ;
  cfg = config.${namespace}.services.caddy;
  inherit (config.${namespace}.system.networking.dns) zone;
  inherit (config.services.caddy) logDir;

  inZone = name: name == zone || hasSuffix ".${zone}" name;

  commonSiteConfig = ''
    tls {
      protocols tls1.3
    }
    encode zstd gzip
    header {
      X-Frame-Options DENY
      X-Content-Type-Options nosniff
      Referrer-Policy strict-origin-when-cross-origin
      Strict-Transport-Security "max-age=31536000; includeSubDomains; preload"
    }
  '';
in
{
  options.${namespace}.services.caddy = {
    enable = mkEnableOption "managing caddy";
    enableDefaultVhost = mkEnableOption "setting up default vhost";
  };

  options.services.caddy.virtualHosts = mkOption {
    type = types.attrsOf (
      types.submodule (
        { name, config, ... }:
        {
          config = mkMerge [
            (mkIf cfg.enable {
              logFormat = mkDefault ''
                output file ${logDir}/access-${replaceStrings [ "/" " " ] [ "_" "_" ] config.hostName}.log {
                  mode 0640
                }
              '';
            })
            (mkIf (cfg.enable && inZone name) {
              useACMEHost = mkDefault name;
              extraConfig = mkBefore commonSiteConfig;
            })
          ];
        }
      )
    );
  };

  config = mkIf cfg.enable {
    ${namespace} = {
      services.acme.enable = true;
      system.networking.firewall.public.caddy = {
        tcp = [
          443
          80
        ];
        udp = [ 443 ];
      };
    };

    services.caddy = {
      enable = true;

      virtualHosts.${zone} = mkIf cfg.enableDefaultVhost {
        serverAliases = [ "www.${zone}" ];
        extraConfig = ''
          @root path /
          handle @root {
            header Content-Type text/html
            respond "<html><body>Hello there!</body></html>"
          }
          handle {
            respond 404
          }
        '';
      };
    };

    security.acme.certs.${zone} = mkIf cfg.enableDefaultVhost {
      extraDomainNames = [ "www.${zone}" ];
    };
  };
}
