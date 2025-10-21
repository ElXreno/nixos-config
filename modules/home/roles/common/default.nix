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
    enable = mkEnableOption "Whether or not to enable common configuration.";
  };

  config = mkIf cfg.enable {
    ${namespace} = {
      programs = {
        direnv.enable = true;
        fish.enable = true;
        git.enable = true;
        helix.enable = true;
        htop.enable = true;
        nix-index.enable = true;
        starship.enable = true;
      };
    };

    # Extra default home stuff
    systemd.user.startServices = "sd-switch";
    xdg = {
      mimeApps.enable = true;
      configFile."mimeapps.list".force = true;
    };
  };
}
