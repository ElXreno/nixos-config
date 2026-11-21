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
  ports = config.${namespace}.ports;
  key = config.clan.core.vars.generators.ripe-atlas.files;
in
{
  options.${namespace}.services.ripe-atlas = {
    enable = mkEnableOption "managing ripe-atlas";
  };

  config = mkIf cfg.enable {
    ${namespace} = {
      system = {
        virtualisation.podman.enable = true;
        impermanence.directories = [ "/var/spool/ripe-atlas" ];
      };
      ports = {
        ripe-atlas-httppost = 18090;
        ripe-atlas-telnetd = 2023;
      };
    };

    clan.core.vars.generators.ripe-atlas = {
      files = {
        probe_key.restartUnits = [ "podman-ripe-atlas.service" ];
        "probe_key.pub".secret = false;
      };
      runtimeInputs = [ pkgs.openssh ];
      script = ''
        ssh-keygen -t ed25519 -N "" -C "${config.clan.core.settings.machine.name}" -f "$out/probe_key"
      '';
    };

    systemd.services.podman-ripe-atlas.preStart = ''
      install -d -m 0755 /run/ripe-atlas/run /var/spool/ripe-atlas
      install -d -m 0770 -o 101 -g 101 /run/ripe-atlas/etc
      install -m 0600 -o 101 -g 101 ${key.probe_key.path} /run/ripe-atlas/etc/probe_key
      install -m 0644 -o 101 -g 101 ${key."probe_key.pub".path} /run/ripe-atlas/etc/probe_key.pub
    '';

    virtualisation.oci-containers.containers.ripe-atlas = {
      image = "docker.io/jamesits/ripe-atlas@sha256:841459abe5aac56833e94832b2460423b5b9532b6420df73bfec54dd3303643b";

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
      extraOptions = [ "--network=host" ];

      environment = {
        RXTXRPT = "yes";
        HTTP_POST_PORT = toString ports.ripe-atlas-httppost;
        TELNETD_PORT = toString ports.ripe-atlas-telnetd;
      };

      volumes = [
        "/run/ripe-atlas/etc:/etc/ripe-atlas"
        "/run/ripe-atlas/run:/run/ripe-atlas"
        "/var/spool/ripe-atlas:/var/spool/ripe-atlas"
      ];
    };
  };
}
