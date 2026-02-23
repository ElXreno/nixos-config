{
  inventory.instances = {
    nixosModules = {
      module.name = "importer";
      roles.default.tags = [ "all" ];
      roles.default.extraModules = [ ../modules ];
    };

    sshd = {
      roles.server.tags = [ "all" ];
      roles.server.settings.authorizedKeys = {
        "root" =
          "ecdsa-sha2-nistp521 AAAAE2VjZHNhLXNoYTItbmlzdHA1MjEAAAAIbmlzdHA1MjEAAACFBAH/QtzrqDZ/isIpMslg5FJvT6BoyeqpmiaDjuzcHaIpTexaq/UK4pAdG7IYvs++6JfdfAToWeU7TnOqRj8eubfFXADNwHC3w7gHjx/w8Yq76gcRG+UU/JtUbphzs2EdWWIupaZV+nFiTSbdGlak4fnnqSLIDhRgNa3pBbvSyf2OdD02bA== elxreno@desktop.local";
      };
    };

    ncps = {
      roles.server = {
        machines.BIMBA.settings = {
          caches = [
            "https://nixos-cache-proxy.elxreno.com"
            "https://nix-community.cachix.org"
            "https://nixpkgs-unfree.cachix.org"
            "https://nix-gaming.cachix.org"
            "https://cuda-maintainers.cachix.org"
            "https://niri.cachix.org"
          ];

          publicKeys = [
            "cache.nixos.org-1:6NCHdD59X431o0gWypbMrAURkbJ16ZPMQFGspcDShjY="
            "nix-community.cachix.org-1:mB9FSh9qf2dCimDSUo8Zy7bkq5CX+/rkCWyvRCYg3Fs="
            "nixpkgs-unfree.cachix.org-1:hqvoInulhbV4nJ9yJOEr+4wxhDV4xq2d1DK7S6Nj6rs="
            "nix-gaming.cachix.org-1:nbjlureqMbRAxR1gJ/f3hxemL9svXaZF/Ees8vCUUs4="
            "cuda-maintainers.cachix.org-1:0dq3bujKpuEPMCX6U4WylrUDZ9JyUG0VpVZa7CNfq5E="
            "niri.cachix.org-1:Wv0OmO7PsuocRKzfDoJ3mulSl7Z6oezYhGhR+3W2964="
          ];
        };
        extraModules = [
          (
            {
              lib,
              ...
            }:
            {
              services.ncps.cache = {
                maxSize = "300G";

                allowDeleteVerb = lib.mkForce false;
                allowPutVerb = lib.mkForce false;

                cdc = {
                  enabled = true;
                };
              };
            }
          )
        ];
      };

      roles.client.tags = [ "all" ];
    };
  };
}
