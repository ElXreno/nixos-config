{
  config,
  namespace,
  lib,
  ...
}:
let
  inherit (lib) mkIf mkEnableOption;
  cfg = config.${namespace}.services.acme;
in
{
  options.${namespace}.services.acme = {
    enable = mkEnableOption "issuing certificates over Cloudflare DNS-01";
  };

  config = mkIf cfg.enable {
    security.acme = {
      acceptTerms = true;
      defaults = {
        email = "elxreno@gmail.com";
        dnsProvider = "cloudflare";
        extraLegoFlags = [ "--dns.propagation.wait=60s" ];
        credentialFiles.CLOUDFLARE_DNS_API_TOKEN_FILE =
          config.clan.core.vars.generators.acme-cloudflare.files.token.path;
      };
    };

    clan.core.vars.generators.acme-cloudflare = {
      prompts.token = {
        description = "Cloudflare API token (Zone:Zone:Read + Zone:DNS:Edit)";
        type = "hidden";
        persist = true;
      };
    };
  };
}
