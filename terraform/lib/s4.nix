{
  lib,
  namespace,
  nixosConfigurations,
}:

let
  machines = lib.filter (m: m.prefixes != [ ]) (
    lib.mapAttrsToList (name: machine: {
      name = lib.toLower name;
      user = "kopia-${lib.toLower name}";
      prefixes = lib.unique (
        map (
          repo: lib.head (lib.splitString "/" repo.prefix)
        ) machine.config.${namespace}.services.kopia.repositories
      );
    }) nixosConfigurations
  );

  zerofs =
    lib.mapAttrsToList
      (name: _: {
        name = lib.toLower name;
        user = "zerofs-${lib.toLower name}";
        prefix = "${lib.toLower name}-";
      })
      (
        lib.filterAttrs (
          _: machine: machine.config.${namespace}.services.zerofs.volumes != { }
        ) nixosConfigurations
      );
in
{
  machines = lib.sort (a: b: a.name < b.name) machines;
  zerofs = lib.sort (a: b: a.name < b.name) zerofs;
}
