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
    {
      checks =
        lib.mapAttrs' (name: toplevel: lib.nameValuePair "nixos-${name}" toplevel) toplevels
        // lib.mapAttrs' (
          name: package: lib.nameValuePair "package-${name}" package
        ) config.legacyPackages.cuda.${clanMeta.name}
        // lib.mapAttrs' (
          name: shell: lib.nameValuePair "devshell-${name}" shell.inputDerivation
        ) config.devShells
        // lib.mapAttrs' (
          name: _: lib.nameValuePair "router-${name}" config.packages."${name}-image"
        ) inputs.self.keeneticConfigurations;
    };
}
