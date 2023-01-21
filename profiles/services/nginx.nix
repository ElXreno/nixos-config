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
      enableReload = true;
      recommendedTlsSettings = true;
      recommendedProxySettings = true;
      recommendedOptimisation = true;
      recommendedBrotliSettings = true;
      virtualHosts = {
        "elxreno.me" = lib.mkIf (config.device == "Nixis-Server") {
          forceSSL = true;
          enableACME = true;
          root = "/var/www/elxreno.me";
        };
      };
    };
  };
}
