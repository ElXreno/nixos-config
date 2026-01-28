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
    enable = mkEnableOption "Whether or not to manage fish." // {
      default = true;
    };
  };

  config = mkIf cfg.enable {
    programs.fish.enable = true;
  };
}
