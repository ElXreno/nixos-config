{
  config,
  lib,
  dns,
  tunnels,
  ...
}:
let
  accountId = config.data.cloudflare_zone.this "account.id";
  tunnelId = name: config.resource.cloudflare_zero_trust_tunnel_cloudflared.${name} "id";
  token =
    name: "jsondecode(base64decode(data.cloudflare_zero_trust_tunnel_cloudflared_token.${name}.token))";
in
{
  resource.cloudflare_zero_trust_tunnel_cloudflared = lib.mapAttrs (name: _: {
    account_id = accountId;
    inherit name;
    config_src = "local";
  }) tunnels;

  data.cloudflare_zero_trust_tunnel_cloudflared_token = lib.mapAttrs (name: _: {
    account_id = accountId;
    tunnel_id = tunnelId name;
  }) tunnels;

  resource.cloudflare_dns_record = lib.listToAttrs (
    lib.concatLists (
      lib.mapAttrsToList (
        name: hostnames:
        map (
          hostname:
          lib.nameValuePair (dns.key hostname "CNAME") {
            zone_id = config.data.cloudflare_zone.this "zone_id";
            name = hostname;
            type = "CNAME";
            content = "${tunnelId name}.cfargotunnel.com";
            proxied = true;
            ttl = 1;
          }
        ) hostnames
      ) tunnels
    )
  );

  output = lib.mapAttrs' (
    name: _:
    lib.nameValuePair "${name}_tunnel_credentials" {
      value = "\${jsonencode({AccountTag = ${token name}.a, TunnelID = ${token name}.t, TunnelSecret = ${token name}.s})}";
      sensitive = true;
    }
  ) tunnels;
}
