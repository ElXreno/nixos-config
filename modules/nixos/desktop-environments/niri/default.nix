{
  config,
  namespace,
  lib,
  pkgs,
  ...
}:
let
  inherit (lib) mkIf mkEnableOption mkForce;
  cfg = config.${namespace}.desktop-environments.niri;
in
{
  options.${namespace}.desktop-environments.niri = {
    enable = mkEnableOption "managing niri";
  };

  config = mkIf cfg.enable {
    ${namespace}.system.impermanence.directories = [
      {
        directory = "/var/cache/tuigreet";
        user = "greeter";
        group = "greeter";
        mode = "0755";
      }
    ];

    services = {
      power-profiles-daemon.enable = true;
      udisks2.enable = true;
      upower.enable = true;
      colord.enable = true;

      greetd = {
        enable = true;
        settings = {
          default_session = {
            command = "${pkgs.cage}/bin/cage -s -d -m last -- ${pkgs.foot}/bin/foot ${pkgs.tuigreet}/bin/tuigreet --time --asterisks --remember --remember-session --sessions ${config.services.displayManager.sessionData.desktops}/share/wayland-sessions";
            user = "greeter";
          };
        };
      };
    };

    security.soteria.enable = true;

    systemd.services.greetd.serviceConfig.Type = mkForce "simple";
    systemd.user.services.niri-flake-polkit.enable = false;

    programs.niri = {
      enable = true;
      package = pkgs.niri-unstable;
    };

    programs.xfconf.enable = true;

    fonts.packages = with pkgs; [
      jetbrains-mono
      nerd-fonts.jetbrains-mono
    ];
  };
}
