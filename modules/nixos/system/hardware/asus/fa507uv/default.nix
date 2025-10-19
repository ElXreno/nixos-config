{
  config,
  namespace,
  lib,
  pkgs,
  ...
}:
let
  inherit (lib) mkIf mkEnableOption;
  cfg = config.${namespace}.system.hardware.asus.fa507uv;

  version = config.boot.kernelPackages.kernel.version;
  majorMinor = lib.versions.majorMinor version;
in
{
  options.${namespace}.system.hardware.asus.fa507uv = {
    enable = mkEnableOption "Whether or not to manage ASUS FA507UV stuff.";
  };

  config = mkIf cfg.enable {
    boot.kernelPatches =
      let
        patchesSrc = pkgs.fetchFromGitHub {
          owner = "CachyOS";
          repo = "kernel-patches";
          rev = "bfcf34bd22aa1fa740c5d60a8f126919cfdacfdf";
          hash = "sha256-YdhrS8JBGnM4BvdkG0MbO8I4dJLmF+RyP7VCRCf7LVQ=";
        };
      in
      [
        {
          name = "asus-armoury";
          patch = "${patchesSrc}/${majorMinor}/0001-asus.patch";
        }
        {
          name = "fa507uv-support";
          patch = ./kernel-patches/0001-platform-x86-asus-armoury-Add-tunings-for-FA507UV-bo.patch;
        }
      ];
  };
}
