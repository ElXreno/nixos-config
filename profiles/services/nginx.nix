{
  inputs,
  pkgs,
  lib,
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
      # www subdomain redirection handled by cloudflare rule
      "elxreno.com" = nginx-common-config // {
        locations."/" = {
          return = "200 '<html><body>Hello there!</body></html>'";
          extraConfig = ''
            default_type text/html;
          '';
        };
      };
    };

    commonHttpConfig =
      let
        realIpsFromList = lib.strings.concatMapStringsSep "\n" (x: "set_real_ip_from ${x};");
        cfipv4 = pkgs.cfipv4;
      in
      ''
        ${realIpsFromList cfipv4}
        real_ip_header CF-Connecting-IP;
        real_ip_recursive on;
      '';

  };

  security.acme = {
    acceptTerms = true;
    defaults.email = "elxreno@gmail.com";
  };
}
