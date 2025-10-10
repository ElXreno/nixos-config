{
  config,
  namespace,
  lib,
  ...
}:
let
  inherit (lib) mkIf mkEnableOption;
  cfg = config.${namespace}.programs.fish;
in
{
  options.${namespace}.programs.fish = {
    enable = mkEnableOption "Whether or not to manage fish.";
  };

  config = mkIf cfg.enable {
    programs.fish.enable = true;
  };
}
