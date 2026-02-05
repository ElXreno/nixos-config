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
    mkForce
    optional
    ;
  cfg = config.${namespace}.system.nix;

  finalPackage = cfg.package.appendPatches [
    ./patches/0001-Support-store-specific-setting-for-HTTP-binary-cache.patch
  ];
in
{
  options.${namespace}.system.nix = {
    enable = mkEnableOption "Whether or not to manage nix." // {
      default = true;
    };
    package = mkPackageOption pkgs.nixVersions "latest" { };
    auto-optimise.enable = mkEnableOption "Whether to enable automatic store optimisation.";
    gc.enable = mkEnableOption "Whether to enable automatic garbage collection.";
  };

  config = mkIf cfg.enable {
    sops.secrets."attic/netrc-file-pull" = {
      sopsFile = "${inputs.self}/secrets/netrc.yaml";
    };

    nix = {
      package = finalPackage;

      settings = mkMerge [
        {
          auto-optimise-store = cfg.auto-optimise.enable;

          builders-use-substitutes = true;

          experimental-features = [
            "nix-command"
            "flakes"
          ]; # TODO: Something adds extra-experimental-features, find who

          extra-sandbox-paths = with config.programs.ccache; optional enable "${cacheDir}";

          trusted-users = [
            "@wheel"
            "elxreno"
          ];

          substituters = mkForce [
            "https://nixos-cache-proxy.elxreno.com?http-version=http3"
            "https://cache.elxreno.com/common"
            "https://nix-community.cachix.org"
          ];
          trusted-public-keys = [
            "common:m1kzZFDmZb76MaOKKGGBkJKZL/Rd8MrlQr+Sk+Q92c4="
            "nix-community.cachix.org-1:mB9FSh9qf2dCimDSUo8Zy7bkq5CX+/rkCWyvRCYg3Fs="
          ];

          netrc-file = config.sops.secrets."attic/netrc-file-pull".path;
        }
        (mkIf (!virtual) {
          min-free = 2 * 1024 * 1024 * 1024; # 2GB
          max-free = 5 * 1024 * 1024 * 1024; # 5GB
        })
      ];

      registry.nixpkgs.flake = inputs.nixpkgs;

      nixPath = mkForce [ "nixpkgs=${inputs.nixpkgs}" ];

      gc = mkIf cfg.gc.enable {
        automatic = true;
        dates = "daily";
        options = "-d --delete-older-than 14d";
      };
    };
  };
}
