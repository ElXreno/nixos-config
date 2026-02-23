{
  config,
  namespace,
  lib,
  ...
}:

let
  inherit (lib) mkIf mkEnableOption;
  cfg = config.${namespace}.programs.ydotool;
in
{
  options.${namespace}.programs.ydotool = {
    enable = mkEnableOption "Whether or not to manage ydotool." // {
      default = with config.${namespace}.desktop-environments; niri.enable || plasma.enable;
    };
  };

  config = mkIf cfg.enable {
    programs.ydotool = {
      enable = true;
    };
  };
}
