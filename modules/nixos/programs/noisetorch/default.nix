{
  config,
  namespace,
  lib,
  ...
}:

let
  inherit (lib) mkIf mkEnableOption;
  cfg = config.${namespace}.programs.noisetorch;
in
{
  options.${namespace}.programs.noisetorch = {
    enable = mkEnableOption "Whether or not to manage noisetorch.";
  };

  config = mkIf cfg.enable {
    programs.noisetorch.enable = true;
  };
}
