{ config, ... }:
let
  baseDomain = "elxreno.me";
  fqdn = "matrix.${baseDomain}";
  baseUrl = "https://${fqdn}";
  clientConfig."m.homeserver".base_url = baseUrl;
  serverConfig."m.server" = "${fqdn}:443";
  mkWellKnown = data: ''
    add_header Content-Type application/json;
    add_header Access-Control-Allow-Origin *;
    return 200 '${builtins.toJSON data}';
  '';
in
{
  sops.secrets = {
    "matrix/db_init_script" = {
      owner = "postgres";
      group = "postgres";
    };

    "matrix/psql_config" = {
      owner = "matrix-synapse";
      group = "matrix-synapse";
    };

    "matrix/registration_shared_secret" = {
      owner = "matrix-synapse";
      group = "matrix-synapse";
    };

    "matrix/coturn" = {
      owner = "turnserver";
      group = "turnserver";
    };
  };

  services.postgresql = {
    enable = true;
    initialScript = config.sops.secrets."matrix/db_init_script".path;
  };

  services.matrix-synapse = {
    enable = true;
    settings = {
      allow_guest_access = false;
      listeners = [
        {
          port = 13748;
          resources = [
            {
              names = [
                "client"
                "federation"
              ];
              compress = true;
            }
          ];
          type = "http";
          tls = false;
          x_forwarded = true;
        }
      ];
      public_baseurl = baseUrl;
      server_name = baseDomain;
      turn_uris = [
        "turn:${fqdn}?transport=udp"
        "turn:${fqdn}?transport=tcp"
      ];
    };
    extraConfigFiles = [
      config.sops.secrets."matrix/psql_config".path
      config.sops.secrets."matrix/registration_shared_secret".path
    ];
  };

  services.coturn = rec {
    enable = true;
    no-cli = true;
    no-tcp-relay = true;
    min-port = 49000;
    max-port = 50000;
    use-auth-secret = true;
    static-auth-secret-file = config.sops.secrets."matrix/coturn".path;
    realm = "${fqdn}";
    cert = "${config.security.acme.certs.${realm}.directory}/full.pem";
    pkey = "${config.security.acme.certs.${realm}.directory}/key.pem";
    extraConfig = ''
      # for debugging
      verbose
      # ban private IP ranges
      no-multicast-peers
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
      denied-peer-ip=240.0.0.0-255.255.255.255
      denied-peer-ip=::1
      denied-peer-ip=64:ff9b::-64:ff9b::ffff:ffff
      denied-peer-ip=::ffff:0.0.0.0-::ffff:255.255.255.255
      denied-peer-ip=100::-100::ffff:ffff:ffff:ffff
      denied-peer-ip=2001::-2001:1ff:ffff:ffff:ffff:ffff:ffff:ffff
      denied-peer-ip=2002::-2002:ffff:ffff:ffff:ffff:ffff:ffff:ffff
      denied-peer-ip=fc00::-fdff:ffff:ffff:ffff:ffff:ffff:ffff:ffff
      denied-peer-ip=fe80::-febf:ffff:ffff:ffff:ffff:ffff:ffff:ffff
    '';
  };

  users.groups.nginx = {
    members = [ "turnserver" ];
  };

  security.acme = {
    acceptTerms = true;
    defaults.email = "elxreno@gmail.com";
    certs.${config.services.coturn.realm} = {
      postRun = "systemctl restart coturn.service";
    };
  };

  services.nginx = {
    enable = true;
    recommendedTlsSettings = true;
    recommendedOptimisation = true;
    recommendedGzipSettings = true;
    recommendedProxySettings = true;
    virtualHosts = {
      ${baseDomain} = {
        enableACME = true;
        forceSSL = true;
        locations."/".extraConfig = ''
          return 404;
        '';
        locations."= /.well-known/matrix/server".extraConfig = mkWellKnown serverConfig;
        locations."= /.well-known/matrix/client".extraConfig = mkWellKnown clientConfig;
      };
      ${fqdn} = {
        enableACME = true;
        forceSSL = true;
        locations."/".extraConfig = ''
          return 404;
        '';
        locations."/_matrix".proxyPass = "http://[::1]:13748";
        locations."/_synapse/client".proxyPass = "http://[::1]:13748";
      };
    };
  };

  networking.firewall = {
    allowedTCPPorts = [
      80
      443
      3478
      5349
    ];
    allowedUDPPorts = [
      3478
      5349
    ];
    allowedUDPPortRanges = [
      {
        from = config.services.coturn.min-port;
        to = config.services.coturn.max-port;
      }
    ];
  };
}
