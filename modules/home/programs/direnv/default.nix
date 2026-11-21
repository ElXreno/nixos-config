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
    enable = mkEnableOption "managing direnv";
  };

  config = mkIf cfg.enable {
    programs = {
      direnv.enable = true;
      bash.enable = true;
    };
  };
}
