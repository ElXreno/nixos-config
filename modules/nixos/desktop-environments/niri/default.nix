{
  config,
  namespace,
  lib,
  pkgs,
  inputs,
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
      upower.enable = true;
      colord.enable = true;

      greetd = {
        enable = true;
        settings = {
          default_session = {
            command = "${pkgs.cage}/bin/cage -s -d -m last -- ${pkgs.foot}/bin/foot ${pkgs.tuigreet}/bin/tuigreet --time --asterisks --remember --remember-session";
            user = "greeter";
          };
        };
      };
    };

    security.soteria.enable = true;
    systemd.user.services.niri-flake-polkit.enable = false;

    programs.niri = {
      enable = true;
      package = inputs.niri.packages.${pkgs.stdenv.hostPlatform.system}.niri-stable.overrideAttrs (old: {
        patches = (old.patches or [ ]) ++ [
          ./niri-scaling-mode.patch
        ];
      });
    };

    fonts.packages = with pkgs; [
      jetbrains-mono
      nerd-fonts.jetbrains-mono
    ];
  };
}
