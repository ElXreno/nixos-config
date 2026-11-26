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
      compsize
      entr
      hyperfine
      nvtopPackages.full
      restic
      screen
      sops
      yq-go

      # Nix stuff
      nix-tree
      nurl
      rom
    ];
  };
}
