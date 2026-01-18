{
  config,
  namespace,
  lib,
  pkgs,
  ...
}:

let
  inherit (lib) mkEnableOption;
  cfg = config.${namespace}.stylix;
in
{
  options.${namespace}.stylix = {
    enable = mkEnableOption "Whether or not to enable stylix.";
  };

  config = {
    stylix = {
      enable = cfg.enable;
      overlays.enable = false;

      base16Scheme = "${pkgs.base16-schemes}/share/themes/ayu-mirage.yaml";
      polarity = "dark";
    };
  };
}
