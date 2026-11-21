{ clanMeta, ... }:
{
  perSystem =
    {
      config,
      pkgs,
      lib,
      ...
    }:
    {
      devShells = {
        default = pkgs.mkShell {
          packages = [ config.packages.clan-cli ];
          inputsFrom = [ config.pre-commit.devShell ];
        };
      }
      // lib.mapAttrs (
        name: _:
        import ../shells/${name}/default.nix {
          inherit pkgs;
          namespace = clanMeta.name;
        }
      ) (builtins.readDir ../shells);
    };
}
