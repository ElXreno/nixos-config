{
  config,
  lib,
  dns,
  mkClanSecret,
  ...
}:
let
  txt = name: content: {
    inherit name;
    type = "TXT";
    content = ''"${content}"'';
    proxied = false;
  };

  zoneRecords = [
    (txt dns.zone "v=spf1 -all")
    (txt "_dmarc.${dns.zone}" "v=DMARC1; p=reject; sp=reject; adkim=s; aspf=s;")
    (txt "*._domainkey.${dns.zone}" "v=DKIM1; p=")
    (txt "_discord.${dns.zone}" "dh=32d179a598384f96ba16defa894bb7ae124597d2")
  ];

  records =
    dns.records
    // lib.listToAttrs (
      map (record: lib.nameValuePair (dns.key record.name record.type) record) zoneRecords
    );
in
{
  terraform.required_providers.cloudflare.source = "cloudflare/cloudflare";

  data.external.cloudflare-api-token = mkClanSecret "cloudflare-api-token";

  provider.cloudflare.api_token = config.data.external.cloudflare-api-token "result.secret";

  data.cloudflare_zone.this.filter.name = dns.zone;

  resource.cloudflare_dns_record = lib.mapAttrs (_: record: {
    zone_id = config.data.cloudflare_zone.this "zone_id";
    inherit (record)
      name
      type
      content
      proxied
      ;
    ttl = 1;
  }) records;
}
