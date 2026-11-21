{ inputs, ... }:
let
  inherit (inputs.nixpkgs) lib;
  toplevels = lib.mapAttrs (
    _name: cfg: cfg.config.system.build.toplevel
  ) inputs.self.nixosConfigurations;
in
{
  flake.hydraJobs = toplevels;

  perSystem =
    { system, ... }:
    {
      checks = lib.mapAttrs' (name: toplevel: lib.nameValuePair "nixos-${name}" toplevel) (
        lib.filterAttrs (_: toplevel: toplevel.system == system) toplevels
      );
    };
}
