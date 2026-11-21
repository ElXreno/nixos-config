{
  config,
  namespace,
  lib,
  ...
}:
let
  inherit (lib) mkIf mkEnableOption mkOption;
  inherit (lib.types) str;
  cfg = config.${namespace}.desktop-environments.plasma-mobile;
in
{
  options.${namespace}.desktop-environments.plasma-mobile = {
    enable = mkEnableOption "managing Plasma Mobile";
    user = mkOption {
      type = str;
      default = config.${namespace}.primaryUser;
      defaultText = lib.literalExpression "config.\${namespace}.primaryUser";
      description = "User logged in automatically into the Plasma Mobile session.";
    };
  };

  config = mkIf cfg.enable {
    services = {
      desktopManager.plasma6.mobile.enable = true;
      displayManager = {
        sddm = {
          enable = true;
          wayland.enable = true;
        };
        autoLogin = {
          enable = true;
          inherit (cfg) user;
        };
      };
    };
  };
}
