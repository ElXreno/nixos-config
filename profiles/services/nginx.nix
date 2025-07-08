{ pkgs, lib, ... }:
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

    recommendedTlsSettings = true;

    recommendedZstdSettings = true;
    recommendedGzipSettings = true;
    recommendedBrotliSettings = true;

    recommendedProxySettings = true;

    recommendedOptimisation = true;

    virtualHosts = {
      # www subdomain redirection handled by cloudflare rule
      "elxreno.com" = {
        enableACME = true;
        forceSSL = true;

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
        fileToList = x: lib.strings.splitString "\n" (builtins.readFile x);
        cfipv4 = fileToList (
          pkgs.fetchurl {
            url = "https://www.cloudflare.com/ips-v4";
            hash = "sha256-8Cxtg7wBqwroV3Fg4DbXAMdFU1m84FTfiE5dfZ5Onns=";
          }
        );
      in
      ''
        ${realIpsFromList cfipv4}
        real_ip_header CF-Connecting-IP;
        real_ip_recursive on;
      '';
  };

  security.acme = {
    acceptTerms = true;
    defaults.server = "https://acme-staging-v02.api.letsencrypt.org/directory";
    defaults.email = "elxreno@gmail.com";
  };
}
