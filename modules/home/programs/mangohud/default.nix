{
  config,
  osConfig,
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
    enable = mkEnableOption "managing MangoHUD" // {
      default = osConfig.${namespace}.programs.steam.enable;
    };
  };

  config = mkIf cfg.enable {
    programs = {
      mangohud = {
        enable = true;
        settings = {
          full = true;
        };
      };
    };
  };
}
