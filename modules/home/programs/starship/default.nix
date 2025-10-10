{
  config,
  namespace,
  lib,
  ...
}:
let
  inherit (lib) mkIf mkEnableOption;
  cfg = config.${namespace}.programs.starship;
in
{
  options.${namespace}.programs.starship = {
    enable = mkEnableOption "Whether or not to manage starship.";
  };

  config = mkIf cfg.enable {
    programs.starship = {
      enable = true;
    };
  };
}
