{
  config,
  namespace,
  lib,
  pkgs,
  ...
}:
let
  inherit (lib) mkIf mkEnableOption;
  cfg = config.${namespace}.common-packages;
in
{
  options.${namespace}.common-packages = {
    enable = mkEnableOption "provisioning common packages";
  };

  config = mkIf cfg.enable {
    home.packages = with pkgs; [
      # CLI Stuff
      age
      aria2
      bc
      binutils
      compsize
      dua
      fd
      file
      iotop
      mtr
      nvtopPackages.full
      pciutils
      python3
      restic
      ripgrep
      screen
      smartmontools
      sops
      tree
      unzip
      usbutils

      # Nix stuff
      nix-tree
      nurl
      rom
    ];
  };
}
