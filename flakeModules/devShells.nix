{ clanMeta, ... }:
{
  perSystem =
    {
      config,
      pkgs,
      inputs',
      lib,
      ...
    }:
    {
      devShells = {
        default = pkgs.mkShell {
          packages = [ (inputs'.clan-core.packages.clan-cli.override { nix = pkgs.nix-optimized; }) ];
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
