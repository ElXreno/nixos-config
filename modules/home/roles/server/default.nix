{
  config,
  namespace,
  lib,
  ...
}:

let
  inherit (lib) mkIf mkEnableOption;
  cfg = config.${namespace}.roles.server;
in
{
  options.${namespace}.roles.server = {
    enable = mkEnableOption "Whether or not to enable server configuration.";
  };

  config = mkIf cfg.enable {
    ${namespace} = {
      common-packages.enable = true;

      programs = {
        direnv.enable = true;
        fish.enable = true;
        git.enable = true;
        helix.enable = true;
        htop.enable = true;
        nix-index.enable = true;
        ssh.enable = true;
        starship.enable = true;
      };
    };
  };
}
