{
  config,
  lib,
  dns,
  ...
}:
let
  zoneId = config.data.cloudflare_zone.this "zone_id";

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
    ];
  };

  resource.cloudflare_zone_setting = lib.mapAttrs (setting_id: value: {
    zone_id = zoneId;
    inherit setting_id value;
  }) settings;
}
