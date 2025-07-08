{
  inputs,
  pkgs,
  ...
}:
let
  nginx-common-config = import inputs.self.nixosProfiles.services.nginx-common-config;
in
{
  networking.firewall = {
    allowedTCPPorts = [
      80
      443
    ];
    allowedUDPPorts = [ 443 ];
  };

  services.nginx = {
    enable = true;

    enableReload = true;
    enableQuicBPF = true;

    package = pkgs.nginxQuic;

    recommendedTlsSettings = true;

    recommendedZstdSettings = true;
    recommendedGzipSettings = true;
    recommendedBrotliSettings = true;

    recommendedProxySettings = true;

    recommendedOptimisation = true;

    clientMaxBodySize = "4g";

    virtualHosts = {
      "elxreno.com" = nginx-common-config // {
        serverAliases = [ "www.elxreno.com" ];
        locations."= /" = {
          return = "200 '<html><body>Hello there!</body></html>'";
          extraConfig = ''
            default_type text/html;
          '';
        };
      };
    };

  };

  security.acme = {
    acceptTerms = true;
    defaults.email = "elxreno@gmail.com";
  };
}
