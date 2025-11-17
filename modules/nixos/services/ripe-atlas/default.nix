{
  config,
  namespace,
  lib,
  pkgs,
  ...
}:

let
  inherit (lib) mkIf mkEnableOption;
  cfg = config.${namespace}.services.ripe-atlas;

  regservers = pkgs.writeScript "reg_servers.sh" ''
    REG_1_HOST=193.0.19.246
    REG_2_HOST=193.0.19.247
  '';
in
{
  options.${namespace}.services.ripe-atlas = {
    enable = mkEnableOption "Whether to manage ripe-atlas.";
  };

  config = mkIf cfg.enable {
    ${namespace}.system.virtualisation.podman.enable = true;

    systemd.services.podman-pre-setup-ripe-atlas = {
      serviceConfig = {
        Type = "oneshot";
      };
      wantedBy = [ "podman-ripe-atlas.service" ];
      path = [ pkgs.podman ];
      script = ''
        mkdir -p /{etc,run,var/spool}/ripe-atlas
      '';
    };

    virtualisation.oci-containers.containers = {
      ripe-atlas = {
        image = "jamesits/ripe-atlas:latest";

        autoStart = true;
        user = "root:root";
        capabilities = {
          all = false;
          NET_RAW = true;
          KILL = true;
          SETUID = true;
          SETGID = true;
          FOWNER = true;
          CHOWN = true;
          DAC_OVERRIDE = true;
        };
        extraOptions = [
          "--network=host"
          # Currently broken
          # "--runtime=runsc"
        ];

        environment = {
          # https://github.com/RIPE-NCC/ripe-atlas-software-probe?tab=readme-ov-file#configuration-options
          RXTXRPT = "yes";
        };

        volumes = [
          "/etc/ripe-atlas:/etc/ripe-atlas"
          "${regservers}:/usr/libexec/ripe-atlas/scripts/reg_servers.sh.prod" # It works only before first `/etc/ripe-atlas` init
          "/run/ripe-atlas:/run/ripe-atlas"
          "/var/spool/ripe-atlas:/var/spool/ripe-atlas"
        ];
      };
    };
  };
}
