{
  config,
  namespace,
  lib,
  pkgs,
  ...
}:

let
  inherit (lib) mkIf mkEnableOption;
  cfg = config.${namespace}.services.dnscrypt-proxy;
in
{
  options.${namespace}.services.dnscrypt-proxy = {
    enable = mkEnableOption "Whether or not to manage dnscrypt-proxy.";
  };

  config = mkIf cfg.enable {
    clan.core.vars.generators.dnscrypt-proxy-local-doh = {
      share = true;

      files."localhost.pem".secret = true;
      files."ca.pem".secret = false;

      script = ''
        # Generate CA
        openssl req -x509 -nodes -newkey rsa:2048 -days 5000 -sha256 \
          -keyout ca-key.pem -out ca-cert.pem \
          -subj "/CN=dnscrypt-proxy Local CA"

        # Generate server key + CSR
        openssl req -nodes -newkey rsa:2048 \
          -keyout server-key.pem -out server.csr \
          -subj "/CN=localhost"

        # Sign server cert with CA (include SANs)
        openssl x509 -req -in server.csr \
          -CA ca-cert.pem -CAkey ca-key.pem -CAcreateserial \
          -out server-cert.pem -days 5000 -sha256 \
          -extfile <(printf "subjectAltName=DNS:localhost,IP:127.0.0.1,IP:::1")

        # Combined key+cert for dnscrypt-proxy
        cat server-key.pem server-cert.pem > "$out/localhost.pem"
        # CA cert for system trust store
        cp ca-cert.pem "$out/ca.pem"
      '';

      runtimeInputs = [ pkgs.openssl ];
    };

    ${namespace}.system.impermanence.directories = [
      {
        directory = "/var/lib/private/dnscrypt-proxy";
        user = "nobody";
        group = "nogroup";
        mode = "0700";
      }
    ];

    security.pki.certificateFiles = [
      config.clan.core.vars.generators."dnscrypt-proxy-local-doh".files."ca.pem".path
    ];

    networking = {
      nameservers = [
        "127.0.0.1"
        "::1"
      ];
      networkmanager.dns = lib.mkForce "systemd-resolved";
    };

    services.resolved.settings.Resolve = {
      DNS = [
        "127.0.0.1:53"
        "[::1]:53"
      ];
      Domains = [ "~." ];
      FallbackDNS = [ "" ];
      DNSOverTLS = false;
    };

    systemd.services.dnscrypt-proxy.serviceConfig.LoadCredential = [
      "localhost.pem:${
        config.clan.core.vars.generators."dnscrypt-proxy-local-doh".files."localhost.pem".path
      }"
    ];

    services.dnscrypt-proxy = {
      enable = true;
      settings = {
        listen_addresses = [
          "127.0.0.1:53"
          "[::1]:53"
        ];

        bootstrap_resolvers = [
          "1.1.1.1:53"
          "8.8.8.8:53"
        ];

        http3 = true;
        http3_probe = true;

        # Enforce good old default
        lb_strategy = "p2";
        lb_estimator = true;

        cache_size = 4096;
        cache_min_ttl = 2400;
        cache_max_ttl = 86400;
        cache_neg_min_ttl = 60;
        cache_neg_max_ttl = 600;

        server_names = [
          "cloudflare"
        ];
        sources = {
          public-resolvers = {
            urls = [
              "https://raw.githubusercontent.com/DNSCrypt/dnscrypt-resolvers/master/v3/public-resolvers.md"
              "https://download.dnscrypt.info/resolvers-list/v3/public-resolvers.md"
              "https://ipv6.download.dnscrypt.info/resolvers-list/v3/public-resolvers.md"
              "https://download.dnscrypt.net/resolvers-list/v3/public-resolvers.md"
            ];
            cache_file = "/var/lib/dnscrypt-proxy/public-resolvers.md";
            minisign_key = "RWQf6LRCGA9i53mlYecO4IzT51TGPpvWucNSCh1CBM0QTaLn73Y7GFO3";
            refresh_delay = 72;
          };
        };

        local_doh = {
          listen_addresses = [ "127.0.0.1:3000" ];
          path = "/dns-query";
          cert_file = "/run/credentials/dnscrypt-proxy.service/localhost.pem";
          cert_key_file = "/run/credentials/dnscrypt-proxy.service/localhost.pem";
        };

        monitoring_ui = {
          enabled = true;
          username = ""; # I don't care
          password = ""; # I don't care
          listen_address = "127.0.0.1:8079";
          privacy_level = 0;
          prometheus_enabled = config.services.victoriametrics.enable;
        };
      };
    };
  };
}
