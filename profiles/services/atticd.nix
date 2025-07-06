{
  config,
  inputs,
  pkgs,
  ...
}:
{
  imports = [
    inputs.self.nixosProfiles.services.postgresql
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
