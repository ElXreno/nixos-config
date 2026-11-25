{
  config,
  lib,
  dns,
  ...
}:
let
  zoneId = config.data.cloudflare_zone.this "zone_id";

  cacheProxyHost =
    (lib.head (lib.importTOML ../cf-workers/nix-cache-proxy/wrangler.toml).routes).pattern;

  cacheProxyPath = lib.concatStringsSep " or " [
    ''http.request.uri.path in {"/" "/nix-cache-info"}''
    ''starts_with(http.request.uri.path, "/u/")''
    ''starts_with(http.request.uri.path, "/cdn-cgi/")''
    ''(ends_with(http.request.uri.path, ".narinfo") and len(http.request.uri.path) eq 41)''
  ];

  ruleset = phase: rules: {
    zone_id = zoneId;
    name = "default";
    kind = "zone";
    inherit phase rules;
  };

  settings = {
    always_use_https = "on";
    automatic_https_rewrites = "off";
    browser_check = "off";
    challenge_ttl = 7200;
    ipv6 = "on";
    opportunistic_onion = "off";
    replace_insecure_js = "on";
    ssl = "full";
  };
in
{
  resource.cloudflare_ruleset = {
    firewall = ruleset "http_request_firewall_custom" [
      {
        ref = "555d10fa161d4ed599f442e478e57017";
        description = "hass from belarus only";
        expression = ''(http.host eq "hass.${dns.zone}" and ip.src.country ne "BY")'';
        action = "block";
        enabled = true;
      }
      {
        ref = "9ef6a279d8243979a67131d56d0821d9";
        description = "nix cache proxy: binary cache paths only";
        expression = ''(http.host eq "${cacheProxyHost}" or starts_with(http.host, "${cacheProxyHost}:")) and (not http.request.method in {"GET" "HEAD"} or not (${cacheProxyPath}))'';
        action = "block";
        enabled = true;
      }
    ];
  };

  resource.cloudflare_zone_setting = lib.mapAttrs (setting_id: value: {
    zone_id = zoneId;
    inherit setting_id value;
  }) settings;
}
