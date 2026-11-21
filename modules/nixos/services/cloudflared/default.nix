{
  config,
  namespace,
  lib,
  ...
}:
let
  inherit (lib)
    attrNames
    mkIf
    mkOption
    toLower
    types
    ;
  cfg = config.${namespace}.services.cloudflared;
in
{
  options.${namespace}.services.cloudflared = {
    ingress = mkOption {
      type = types.lazyAttrsOf types.str;
      default = { };
      example = {
        "app.example.com" = "http://localhost:8080";
      };
      description = ''
        Public host names served through the Cloudflare tunnel of this
        host, each with the local service it reaches. Any entry starts the
        connector; tofu creates the tunnel and its DNS records.
      '';
    };
  };

  config = mkIf (attrNames cfg.ingress != [ ]) {
    clan.core.vars.generators.cloudflared.prompts.credentials = {
      description = "Cloudflare tunnel credentials of this host (tofu output <host>_tunnel_credentials)";
      type = "hidden";
      persist = true;
    };

    services.cloudflared = {
      enable = true;
      tunnels.${toLower config.networking.hostName} = {
        credentialsFile = config.clan.core.vars.generators.cloudflared.files.credentials.path;
        default = "http_status:404";
        inherit (cfg) ingress;
      };
    };
  };
}
