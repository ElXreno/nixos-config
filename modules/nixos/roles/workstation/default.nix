{
  config,
  namespace,
  lib,
  pkgs,
  ...
}:

let
  inherit (lib) mkIf mkEnableOption;
  cfg = config.${namespace}.roles.workstation;
in
{
  options.${namespace}.roles.workstation = {
    enable = mkEnableOption "workstation configuration";
  };

  config = mkIf cfg.enable {
    ${namespace} = {
      programs = {
        haven.enable = true;
        winbox.enable = true;
        wireshark.enable = true;
      };

      services = {
        flatpak.enable = true;
        pipewire.rnnoise.enable = true;
      };

      desktop-environments.niri.enable = true;

      system.hardware = {
        yubikey.enable = true;

        bluetooth.xboxSupport = true;
      };

      system.networking.firewall.public.spotify-connect.tcp = [ 57621 ];

      tailscale.tags = [ "tag:workstation" ];
    };

    environment.systemPackages = [ pkgs.android-tools ];
  };
}
