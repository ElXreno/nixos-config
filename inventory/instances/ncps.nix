{
  inventory.instances.ncps = {
    roles.server = {
      machines.BIMBA.settings = {
        caches = [
          "https://nixos-cache-proxy.elxreno.com"
          "https://cache.nixos-cuda.org"
          "https://nix-community.cachix.org"
          "https://nixpkgs-unfree.cachix.org"
          "https://nix-gaming.cachix.org"
          "https://niri.cachix.org"
          "https://elxreno.cachix.org"
          "https://cache.numtide.com"
        ];

        openFirewall = false;

        publicKeys = [
          "cache.nixos.org-1:6NCHdD59X431o0gWypbMrAURkbJ16ZPMQFGspcDShjY="
          "cache.nixos-cuda.org:74DUi4Ye579gUqzH4ziL9IyiJBlDpMRn9MBN8oNan9M="
          "nix-community.cachix.org-1:mB9FSh9qf2dCimDSUo8Zy7bkq5CX+/rkCWyvRCYg3Fs="
          "nixpkgs-unfree.cachix.org-1:hqvoInulhbV4nJ9yJOEr+4wxhDV4xq2d1DK7S6Nj6rs="
          "nix-gaming.cachix.org-1:nbjlureqMbRAxR1gJ/f3hxemL9svXaZF/Ees8vCUUs4="
          "niri.cachix.org-1:Wv0OmO7PsuocRKzfDoJ3mulSl7Z6oezYhGhR+3W2964="
          "elxreno.cachix.org-1:ozSPSY5S3/TpbcXi+/DdtSj1JlK3CPz3G+F92yRBXDQ="
          "niks3.numtide.com-1:DTx8wZduET09hRmMtKdQDxNNthLQETkc/yaX7M4qK0g="
        ];
      };
      extraModules = [
        (
          {
            config,
            lib,
            namespace,
            ...
          }:
          let
            goMemLimitMiB = lib.max 512 (config.${namespace}.facts.memory.totalMiB * 5 / 100);
          in
          {
            services.ncps.cache = {
              maxSize = "300G";

              databaseURL = "postgresql:///ncps?host=/run/postgresql";

              allowDeleteVerb = lib.mkForce false;
              allowPutVerb = lib.mkForce false;

              cdc.enabled = false;
            };

            systemd.services.ncps = {
              after = [ "postgresql.service" ];
              requires = [ "postgresql.service" ];

              environment.GOMEMLIMIT = "${toString goMemLimitMiB}MiB";

              serviceConfig.OOMScoreAdjust = -500;
            };

            ${namespace} = {
              ports.ncps = 8501;
              services.postgresql = {
                enable = true;
                databases.ncps = { };
              };
            };
          }
        )
      ];
    };

    roles.client.tags = [ "all" ];
  };
}
