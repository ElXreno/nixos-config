{
  config,
  namespace,
  lib,
  ...
}:
let
  inherit (lib) mkIf mkEnableOption;
  cfg = config.${namespace}.programs.kitty;
in
{
  options.${namespace}.programs.kitty = {
    enable = mkEnableOption "Whether or not to manage kitty.";
  };

  config = mkIf cfg.enable {
    programs.kitty = {
      enable = true;
    };
  };
}
