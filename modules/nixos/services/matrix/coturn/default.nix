{
  config,
  namespace,
  lib,
  pkgs,
  ...
}:

let
  inherit (lib) mkIf;
  cfg = config.${namespace}.services.matrix;

  baseDomain = cfg.serverName;
  turnHost = "turn.${baseDomain}";
  certDir = "/var/lib/acme/${turnHost}";
  coturn = config.services.coturn;
in
{
  config = mkIf (cfg.enable && cfg.voip.enable) {
    clan.core.vars.generators.matrix-coturn-secret = {
      files.secret = {
        secret = true;
        restartUnits = [
          "coturn.service"
          "matrix-synapse.service"
        ];
      };
      runtimeInputs = [ pkgs.openssl ];
      script = ''
        openssl rand -hex 32 | tr -d '\n' > "$out/secret"
      '';
    };

    security.acme.certs."${turnHost}".reloadServices = [ "coturn.service" ];

    users.users.turnserver.extraGroups = [
      "acme"
      "caddy"
    ];

    services.coturn = {
      enable = true;
      no-cli = true;
      use-auth-secret = true;
      static-auth-secret-file = "/run/credentials/coturn.service/static-auth-secret";
      realm = turnHost;
      cert = "${certDir}/fullchain.pem";
      pkey = "${certDir}/key.pem";
      secure-stun = true;
      no-tcp-relay = true;
      min-port = 52000;
      max-port = 65535;
      extraConfig = ''
        denied-peer-ip=0.0.0.0-0.255.255.255
        denied-peer-ip=10.0.0.0-10.255.255.255
        denied-peer-ip=100.64.0.0-100.127.255.255
        denied-peer-ip=127.0.0.0-127.255.255.255
        denied-peer-ip=169.254.0.0-169.254.255.255
        denied-peer-ip=172.16.0.0-172.31.255.255
        denied-peer-ip=192.0.0.0-192.0.0.255
        denied-peer-ip=192.0.2.0-192.0.2.255
        denied-peer-ip=192.88.99.0-192.88.99.255
        denied-peer-ip=192.168.0.0-192.168.255.255
        denied-peer-ip=198.18.0.0-198.19.255.255
        denied-peer-ip=198.51.100.0-198.51.100.255
        denied-peer-ip=203.0.113.0-203.0.113.255
        denied-peer-ip=224.0.0.0-239.255.255.255
        denied-peer-ip=240.0.0.0-255.255.255.255
        denied-peer-ip=::1
        denied-peer-ip=64:ff9b::-64:ff9b::ffff:ffff
        denied-peer-ip=::ffff:0.0.0.0-::ffff:255.255.255.255
        denied-peer-ip=100::-100::ffff:ffff:ffff:ffff
        denied-peer-ip=2001::-2001:1ff:ffff:ffff:ffff:ffff:ffff:ffff
        denied-peer-ip=2002::-2002:ffff:ffff:ffff:ffff:ffff:ffff:ffff
        denied-peer-ip=fc00::-fdff:ffff:ffff:ffff:ffff:ffff:ffff:ffff
        denied-peer-ip=fe80::-febf:ffff:ffff:ffff:ffff:ffff:ffff:ffff
        user-quota=12
        total-quota=1200
      '';
    };

    systemd.services.coturn.serviceConfig.LoadCredential = [
      "static-auth-secret:${config.clan.core.vars.generators.matrix-coturn-secret.files.secret.path}"
    ];

    services.matrix-synapse.settings = {
      turn_uris = [
        "turn:${turnHost}:${toString coturn.listening-port}?transport=udp"
        "turn:${turnHost}:${toString coturn.listening-port}?transport=tcp"
        "turns:${turnHost}:${toString coturn.tls-listening-port}?transport=udp"
        "turns:${turnHost}:${toString coturn.tls-listening-port}?transport=tcp"
      ];
      turn_shared_secret_path = "/run/credentials/matrix-synapse.service/coturn-shared-secret";
      turn_user_lifetime = 86400000;
      turn_allow_guests = false;
    };

    systemd.services.matrix-synapse.serviceConfig.LoadCredential = [
      "coturn-shared-secret:${config.clan.core.vars.generators.matrix-coturn-secret.files.secret.path}"
    ];

    ${namespace} = {
      ports = {
        coturn-stun = coturn.listening-port;
        coturn-stun-alt = coturn.alt-listening-port;
        coturn-turns = coturn.tls-listening-port;
        coturn-turns-alt = coturn.alt-tls-listening-port;
      };

      system.networking.firewall.public.coturn = rec {
        tcp = [
          coturn.listening-port
          coturn.alt-listening-port
          coturn.tls-listening-port
          coturn.alt-tls-listening-port
        ];
        udp = tcp;
        udpRanges = [
          {
            from = coturn.min-port;
            to = coturn.max-port;
          }
        ];
      };
    };

    services.caddy.virtualHosts."${turnHost}".extraConfig = ''
      respond 204
    '';
  };
}
