{ config, lib, ... }:
let
  missing = lib.subtractLists (import ./_outputs.nix) (lib.attrNames config.flake);
in
{
  perSystem =
    { pkgs, ... }:
    {
      checks.flake-outputs =
        if missing == [ ] then
          pkgs.emptyFile
        else
          throw "flake outputs not listed in flakeModules/_outputs.nix: ${toString missing}";
    };
}
