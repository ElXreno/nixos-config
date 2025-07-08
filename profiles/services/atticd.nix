{
  config,
  inputs,
  pkgs,
  ...
}:
let
  baseDomain = "elxreno.com";

  cacheEndpoint = "cache.${baseDomain}";

  nginx-common-config = import inputs.self.nixosProfiles.services.nginx-common-config;
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

  services.nginx.virtualHosts = {
    "${cacheEndpoint}" = nginx-common-config // {
      locations."/" = {
        proxyPass = "http://localhost:18080";
      };
    };
  };

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
