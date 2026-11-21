{
  config,
  namespace,
  lib,
  pkgs,
  ...
}:
let
  inherit (lib)
    mkIf
    mkEnableOption
    mkOption
    types
    ;
  cfg = config.${namespace}.programs.telegram-desktop;

  themed = pkgs.${namespace}.telegram-desktop-base16.override {
    colors = config.lib.stylix.colors.withHashtag;
    inherit (cfg) chromaPull;
  };
in
{
  options.${namespace}.programs.telegram-desktop = {
    enable = mkEnableOption "managing Telegram Desktop";

    chromaPull = mkOption {
      type = types.float;
      default = 0.6;
      description = "How far accent chroma is pulled towards the palette. Only light accents are lifted; dark fills are never saturated beyond the bundled theme's.";
    };
  };

  config = mkIf cfg.enable {
    home.packages = [ (if config.stylix.enable then themed else pkgs.telegram-desktop) ];
  };
}
