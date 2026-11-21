{
  config,
  namespace,
  lib,
  ...
}:
let
  inherit (lib) mkIf mkEnableOption;

  inherit (config.lib.stylix.colors.withHashtag) base04 base07;
  cfg = config.${namespace}.programs.kitty;
in
{
  options.${namespace}.programs.kitty = {
    enable = mkEnableOption "managing kitty";
  };

  config = mkIf cfg.enable {
    programs.kitty = {
      enable = true;
      settings = {
        auto_reload_config = -1;
        text_composition_strategy = "1.3 15";
      };

      extraConfig = lib.mkAfter ''
        selection_foreground ${base07}
        color8 ${base04}
      '';
    };
  };
}
