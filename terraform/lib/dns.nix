{
  lib,
  namespace,
  nixosConfigurations,
}:

let
  key = name: type: lib.replaceStrings [ "." "*" ] [ "_" "wildcard" ] "${name}_${type}";

  machineRecords =
    machine:
    let
      dns = machine.config.${namespace}.system.networking.dns;
    in
    lib.concatLists (
      lib.mapAttrsToList (
        name: record:
        if record.cname != null then
          [
            {
              inherit name;
              inherit (record) proxied;
              type = "CNAME";
              content = record.cname;
            }
          ]
        else
          lib.optional (dns.addresses.v4 != null) {
            inherit name;
            inherit (record) proxied;
            type = "A";
            content = dns.addresses.v4;
          }
          ++ lib.optional (dns.addresses.v6 != null) {
            inherit name;
            inherit (record) proxied;
            type = "AAAA";
            content = dns.addresses.v6;
          }
      ) dns.records
    );

  zones = lib.unique (
    lib.mapAttrsToList (
      _: machine: machine.config.${namespace}.system.networking.dns.zone
    ) nixosConfigurations
  );
in
{
  zone = lib.head zones;
  inherit key;
  records = lib.listToAttrs (
    map (record: lib.nameValuePair (key record.name record.type) record) (
      lib.concatMap machineRecords (lib.attrValues nixosConfigurations)
    )
  );
}
