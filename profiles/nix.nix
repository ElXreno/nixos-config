{ config, inputs, lib, ... }: {
  nix = {
    settings = {
      auto-optimise-store = config.deviceSpecific.isServer;

      builders-use-substitutes = true;

      experimental-features = [ "nix-command" "flakes" ];

      extra-sandbox-paths = [
        (lib.mkIf config.programs.ccache.enable
          "${config.programs.ccache.cacheDir}")
      ];

      trusted-users = [ "elxreno" ]
        ++ lib.optional config.services.hydra.enable "hydra";

      substituters = [ "https://elxreno.cachix.org" ];
      trusted-public-keys =
        [ "elxreno.cachix.org-1:ozSPSY5S3/TpbcXi+/DdtSj1JlK3CPz3G+F92yRBXDQ=" ];
    } // lib.optionalAttrs config.deviceSpecific.isServer {
      min-free = 2 * 1024 * 1024 * 1024; # 2GB
      max-free = 5 * 1024 * 1024 * 1024; # 5GB
    } // lib.optionalAttrs (config.device != "flamingo") {
      substituters = [ "https://flamingo.angora-ide.ts.net/cache/elxreno" ];
      trusted-public-keys =
        [ "elxreno:tZ38Gs0Wmc5fpulInZeahMwgFyFzFBRMTxFlx4LwRVE=" ];
    };

    daemonCPUSchedPolicy = "batch";
    daemonIOSchedClass = "idle";
    daemonIOSchedPriority = 7;

    registry.nixpkgs.flake = inputs.nixpkgs;

    nixPath = lib.mkForce [ "nixpkgs=${inputs.nixpkgs}" ];

    gc = lib.mkIf config.deviceSpecific.isServer {
      automatic = true;
      dates = "daily";
      options = "-d --delete-older-than 3d";
    };
  };
}
