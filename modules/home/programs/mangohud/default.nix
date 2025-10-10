{
  config,
  namespace,
  lib,
  ...
}:

let
  inherit (lib) mkIf mkEnableOption;
  cfg = config.${namespace}.programs.mangohud;
in
{
  options.${namespace}.programs.mangohud = {
    enable = mkEnableOption "Whether or not to manage MangoHUD.";
  };

  config = mkIf cfg.enable {
    programs = {
      mangohud = {
        enable = true;
        settings = {
          full = true;
          font_size = 18;
        };
      };
    };
  };
}
