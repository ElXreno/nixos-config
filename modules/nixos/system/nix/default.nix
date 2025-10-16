{
  config,
  namespace,
  virtual,
  lib,
  pkgs,
  inputs,
  ...
}:
let
  inherit (lib)
    mkIf
    mkEnableOption
    mkPackageOption
    mkMerge
    optional
    ;
  cfg = config.${namespace}.system.nix;
in
{
  options.${namespace}.system.nix = {
    enable = mkEnableOption "Whether or not to manage nix." // {
      default = true;
    };
    package = mkPackageOption pkgs.nixVersions "nix_2_31" { };
    auto-optimise.enable = mkEnableOption "Whether to enable automatic store optimisation.";
    gc.enable = mkEnableOption "Whether to enable automatic garbage collection.";
  };

  config = mkIf cfg.enable {
    nix = {
      inherit (cfg) package;

      settings = mkMerge [
        {
          auto-optimise-store = cfg.auto-optimise.enable;

          builders-use-substitutes = true;

          experimental-features = [
            "nix-command"
            "flakes"
          ]; # TODO: Something adds extra-experimental-features, find who

          extra-sandbox-paths = optional config.programs.ccache.enable "${config.programs.ccache.cacheDir}";

          trusted-users = [
            "@wheel"
            "elxreno"
          ];

          substituters = [
            "https://cache.elxreno.com:8443/common"
            "https://nix-community.cachix.org"
          ];
          trusted-public-keys = [
            "common:m1kzZFDmZb76MaOKKGGBkJKZL/Rd8MrlQr+Sk+Q92c4="
            "nix-community.cachix.org-1:mB9FSh9qf2dCimDSUo8Zy7bkq5CX+/rkCWyvRCYg3Fs="
          ];
        }
        (mkIf (!virtual) {
          min-free = 2 * 1024 * 1024 * 1024; # 2GB
          max-free = 5 * 1024 * 1024 * 1024; # 5GB
        })
      ];

      daemonCPUSchedPolicy = "idle";
      daemonIOSchedClass = "idle";
      daemonIOSchedPriority = 7;

      registry.nixpkgs.flake = inputs.nixpkgs;

      nixPath = lib.mkForce [ "nixpkgs=${inputs.nixpkgs}" ];

      gc = mkIf cfg.gc.enable {
        automatic = true;
        dates = "daily";
        options = "-d --delete-older-than 3d";
      };
    };
  };
}
