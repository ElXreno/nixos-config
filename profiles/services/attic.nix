{ config, inputs, pkgs, lib, ... }: {
  imports =
    [ inputs.attic.nixosModules.atticd inputs.self.nixosProfiles.postgresql ];

  environment.systemPackages = [ inputs.attic.packages.${pkgs.system}.attic ];

  services.atticd = {
    enable = true;
    credentialsFile = config.sops.secrets."attic/env".path;

    settings = {
      listen = "[::]:8080";

      database = lib.mkForce
        { }; # db url set via env variable (ATTIC_SERVER_DATABASE_URL)

      chunking = {
        nar-size-threshold = 64 * 1024; # 64 KiB
        min-size = 16 * 1024; # 16 KiB
        avg-size = 64 * 1024; # 64 KiB
        max-size = 256 * 1024; # 256 KiB
      };

      garbage-collection = { default-retention-period = "3 months"; };
    };
  };

  services.postgresql = {
    # TODO: Replace with atticd-init systemd service
    initialScript = config.sops.secrets."attic/db_init_script".path;
  };

  sops.secrets."attic/env" = { };
  sops.secrets."attic/db_init_script" = {
    owner = "postgres";
    group = "postgres";
  };
}
