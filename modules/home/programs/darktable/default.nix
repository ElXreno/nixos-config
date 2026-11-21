{
  config,
  namespace,
  lib,
  pkgs,
  ...
}:

let
  inherit (lib) mkIf mkEnableOption;
  cfg = config.${namespace}.programs.darktable;
in
{
  options.${namespace}.programs.darktable = {
    enable = mkEnableOption "managing darktable";
  };

  config = mkIf cfg.enable {
    home.packages = [ pkgs.darktable ];
  };
}
