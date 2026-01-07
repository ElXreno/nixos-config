{
  config,
  namespace,
  lib,
  pkgs,
  ...
}:
let
  inherit (lib) mkIf mkEnableOption;
  cfg = config.${namespace}.desktop-environments.sway;
in
{
  options.${namespace}.desktop-environments.sway = {
    enable = mkEnableOption "Whether or not to manage sway.";
  };

  config = mkIf cfg.enable {
    services = {
      power-profiles-daemon.enable = true;
      udisks2.enable = true;
      colord.enable = true;
    };

    programs.sway.enable = true;

    services.greetd = {
      enable = true;
      settings = {
        default_session = {
          command = "${pkgs.cage}/bin/cage -s -d -m last -- ${pkgs.foot}/bin/foot ${pkgs.tuigreet}/bin/tuigreet --time --asterisks --remember --remember-session";
          user = "greeter";
        };
      };
    };

    fonts.packages = with pkgs; [
      jetbrains-mono
      nerd-fonts.jetbrains-mono
    ];
  };
}
