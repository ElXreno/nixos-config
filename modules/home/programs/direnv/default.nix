{
  config,
  namespace,
  lib,
  ...
}:

let
  inherit (lib) mkIf mkEnableOption;
  cfg = config.${namespace}.programs.direnv;
in
{
  options.${namespace}.programs.direnv = {
    enable = mkEnableOption "Whether or not to manage direnv.";
  };

  config = mkIf cfg.enable {
    programs = {
      direnv = {
        enable = true;
        nix-direnv = {
          enable = true;
        };
      };
      bash.enable = true;
    };
  };
}
