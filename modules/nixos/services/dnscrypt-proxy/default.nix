{
  config,
  namespace,
  lib,
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
    networking.networkmanager.dns = "none";

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
          "google"
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
