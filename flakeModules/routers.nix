{ inputs, lib, ... }:
let
  inherit (lib) concatMapAttrs filterAttrs mapAttrs;

  names = filterAttrs (_: type: type == "directory") (builtins.readDir ../routers);

  mkRouters =
    buildPlatform:
    mapAttrs (
      name: _:
      inputs.nix-keenetic.lib.keeneticSystem {
        inherit buildPlatform;
        modules = [ ../routers/${name} ];
      }
    ) names;
in
{
  flake.keeneticConfigurations = mkRouters "x86_64-linux";

  perSystem =
    { system, ... }:
    {
      packages = concatMapAttrs (name: router: {
        "${name}-image" = router.config.build.image;
        "${name}-deploy" = router.config.build.deploy;
      }) (mkRouters system);
    };
}
