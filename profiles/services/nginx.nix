{ config, lib, ... }:

{
  security.acme = {
    acceptTerms = true;
    defaults.email = "elxreno@gmail.com";
  };

  networking.firewall.allowedTCPPorts = [
    80
    443
  ];

  services = {
    nginx = {
      enable = true;
      recommendedTlsSettings = true;
      recommendedProxySettings = true;
      recommendedOptimisation = true;
      recommendedGzipSettings = true;
      virtualHosts = {
        "elxreno.me" = lib.mkIf (config.device == "Nixis-Server") {
          forceSSL = true;
          enableACME = true;
          root = "/var/www/elxreno.me";
        };
        "funquiz.elxreno.me" = lib.mkIf (config.device == "Nixis-Server" && config.services.fun-quiz-api.enable) {
          forceSSL = true;
          enableACME = true;
          locations."/" = {
            proxyPass = "http://localhost:${toString config.services.fun-quiz-api.port}";
            extraConfig = "proxy_pass_header Authorization;";
          };
        };
      };
    };
  };
}
