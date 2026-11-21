{
  config,
  lib,
  tailnet,
  mkClanSecret,
  ...
}:
{
  terraform.required_providers.tailscale.source = "tailscale/tailscale";

  data.external.tailscale-oauth-client-id = mkClanSecret "tailscale-oauth-client-id";
  data.external.tailscale-oauth-client-secret = mkClanSecret "tailscale-oauth-client-secret";

  provider.tailscale = {
    oauth_client_id = config.data.external.tailscale-oauth-client-id "result.secret";
    oauth_client_secret = config.data.external.tailscale-oauth-client-secret "result.secret";
  };

  resource.tailscale_service = lib.mapAttrs (
    name: svc:
    {
      name = "svc:${name}";
      inherit (svc) tags ports;
      depends_on = [ "tailscale_acl.this" ];
    }
    // lib.optionalAttrs (svc.comment != "") { inherit (svc) comment; }
  ) tailnet.services;

  output = lib.mapAttrs' (
    name: _: lib.nameValuePair "${name}_addrs" { value = "\${tailscale_service.${name}.addrs}"; }
  ) tailnet.services;
}
