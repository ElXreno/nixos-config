{ config, inputs, pkgs, ... }: {
  imports = [ inputs.attic.nixosModules.atticd ];

  environment.systemPackages = [ inputs.attic.packages.${pkgs.system}.attic ];

  services.atticd = {
    enable = true;
    credentialsFile = config.sops.secrets.attic.path;

    settings = {
      listen = "[::]:8080";

      chunking = {
        nar-size-threshold = 64 * 1024; # 64 KiB
        min-size = 16 * 1024; # 16 KiB
        avg-size = 64 * 1024; # 64 KiB
        max-size = 256 * 1024; # 256 KiB
      };
    };
  };

  sops.secrets.attic = { };
}
