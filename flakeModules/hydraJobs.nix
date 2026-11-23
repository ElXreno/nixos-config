{ inputs, clanMeta, ... }:
let
  inherit (inputs.nixpkgs) lib;
  toplevels = lib.mapAttrs (
    _name: cfg: cfg.config.system.build.toplevel
  ) inputs.self.nixosConfigurations;
in
{
  flake.hydraJobs = toplevels;

  perSystem =
    { config, ... }:
    let
      updatable = lib.filterAttrs (
        _: package:
        let
          position = builtins.unsafeGetAttrPos "updateScript" package;
        in
        package ? updateScript && position != null && lib.hasPrefix "${inputs.self}/packages/" position.file
      ) config.packages;
    in
    {
      checks =
        lib.mapAttrs' (name: toplevel: lib.nameValuePair "nixos-${name}" toplevel) toplevels
        // lib.mapAttrs' (
          name: _: lib.nameValuePair "package-${name}" config.legacyPackages.cuda.${clanMeta.name}.${name}
        ) updatable
        // lib.mapAttrs' (
          name: shell: lib.nameValuePair "devshell-${name}" shell.inputDerivation
        ) config.devShells
        // lib.mapAttrs' (
          name: _: lib.nameValuePair "router-${name}" config.packages."${name}-image"
        ) inputs.self.keeneticConfigurations;
    };
}
