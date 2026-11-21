{
  config,
  namespace,
  lib,
  ...
}:

let
  inherit (lib) mkIf mkEnableOption;
  cfg = config.${namespace}.roles.common;
in
{
  options.${namespace}.roles.common = {
    enable = mkEnableOption "common configuration";
  };

  config = mkIf cfg.enable {
    ${namespace} = {
      programs = {
        direnv.enable = true;
        fish.enable = true;
        git.enable = true;
        helix.enable = true;
        htop.enable = true;
        lazygit.enable = true;
        nix-index.enable = true;
        starship.enable = true;
      };
    };
  };
}
