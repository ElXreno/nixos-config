{
  config,
  inputs,
  lib,
  ...
}:
{
  nix = {
    settings =
      {
        auto-optimise-store = config.deviceSpecific.isServer;

        builders-use-substitutes = true;

        experimental-features = [
          "nix-command"
          "flakes"
        ];

        extra-sandbox-paths = [
          (lib.mkIf config.programs.ccache.enable "${config.programs.ccache.cacheDir}")
        ];

        trusted-users = [ "elxreno" ];

        substituters = lib.mkForce [
          "https://cache.nixos.org"
          "https://cache.elxreno.com/common"
          "https://nix-community.cachix.org"
        ];
        trusted-public-keys = [
          "common:d1flxCApyS+J1zmvXwPVvtOg89nXd1p4XJHQLePfMFY="
          "nix-community.cachix.org-1:mB9FSh9qf2dCimDSUo8Zy7bkq5CX+/rkCWyvRCYg3Fs="
        ];
      }
      // lib.optionalAttrs config.deviceSpecific.isServer {
        min-free = 2 * 1024 * 1024 * 1024; # 2GB
        max-free = 5 * 1024 * 1024 * 1024; # 5GB
      };

    daemonCPUSchedPolicy = "idle";
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
