{
  config,
  osConfig,
  namespace,
  lib,
  ...
}:
let
  cfg = config.${namespace}.programs.flydigictl;
in
{
  options.${namespace}.programs.flydigictl = {
    enable = lib.mkEnableOption "flydigictl" // {
      default = osConfig.${namespace}.services.flydigictl.enable;
    };
  };

  config = lib.mkIf cfg.enable {
    programs.flydigictl.enable = true;
  };
}
