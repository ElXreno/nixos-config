{
  config,
  inputs,
  pkgs,
  lib,
  ...
}:
let
  baseDomain = "elxreno.com";

  cacheEndpoint = "cache.${baseDomain}";
  cacheNoProxyEndpoint = "cache-noproxy.${baseDomain}";
in
{
  imports = [
    inputs.self.nixosProfiles.services.postgresql
    inputs.self.nixosProfiles.services.nginx
  ];

  environment.systemPackages = with pkgs; [ attic-client ];

  services.atticd = {
    enable = true;
    environmentFile = config.sops.secrets."attic/env".path;

    settings = {
      listen = "[::]:18080";

      # https://github.com/zhaofengli/attic/issues/48#issuecomment-1636248282
      api-endpoint = "https://${cacheNoProxyEndpoint}/";
      substituter-endpoint = "https://${cacheEndpoint}/";

      database.url = "postgresql://atticd?host=/run/postgresql";

      chunking = {
        nar-size-threshold = 64 * 1024; # 64 KiB
        min-size = 16 * 1024; # 16 KiB
        avg-size = 64 * 1024; # 64 KiB
        max-size = 256 * 1024; # 256 KiB
      };

      garbage-collection = {
        default-retention-period = "3 months";
      };
    };
  };

  services.nginx.virtualHosts =
    let
      mkAtticHost = _: {
        forceSSL = true;
        enableACME = true;
        locations."/" = {
          proxyPass = "http://localhost:18080";
        };
      };
    in
    lib.genAttrs [ cacheEndpoint cacheNoProxyEndpoint ] mkAtticHost;

  services.postgresql = {
    ensureDatabases = [ "atticd" ];
    ensureUsers = [
      {
        name = "atticd";
        ensureDBOwnership = true;
      }
    ];
  };

  sops.secrets."attic/env" = { };
}
