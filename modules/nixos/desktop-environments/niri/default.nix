{
  config,
  namespace,
  lib,
  pkgs,
  ...
}:
let
  inherit (lib) mkIf mkEnableOption;
  cfg = config.${namespace}.desktop-environments.niri;
in
{
  options.${namespace}.desktop-environments.niri = {
    enable = mkEnableOption "Whether or not to manage niri.";
  };

  config = mkIf cfg.enable {
    services = {
      power-profiles-daemon.enable = true;
      udisks2.enable = true;
      colord.enable = true;
    };

    services.greetd = {
      enable = true;
      settings = {
        default_session = {
          command = "${pkgs.cage}/bin/cage -s -d -m last -- ${pkgs.foot}/bin/foot ${pkgs.tuigreet}/bin/tuigreet --time --asterisks --remember --remember-session";
          user = "greeter";
        };
      };
    };

    programs.niri = {
      enable = true;
    };

    fonts.packages = with pkgs; [
      jetbrains-mono
      nerd-fonts.jetbrains-mono
    ];

    xdg.portal.extraPortals = with pkgs; [ xdg-desktop-portal-wlr ];
  };
}
