{ inputs, clanMeta, ... }:
{
  perSystem =
    { system, lib, ... }:
    let
      customNixpkgs =
        (import ../nixpkgs.nix {
          inherit inputs;
          namespace = clanMeta.name;
        })
          system;
      inherit (customNixpkgs) pkgs;
    in
    {
      _module.args.pkgs = pkgs;
      clan.pkgs = pkgs;

      packages = customNixpkgs.packages // {
        install-iso =
          (lib.nixosSystem {
            inherit system;
            modules = [
              ../installIso/default.nix
              { nixpkgs.pkgs = pkgs; }
            ];
            specialArgs = { inherit inputs; };
          }).config.system.build.isoImage;

        push-attic-ci-token = pkgs.writeShellApplication {
          name = "push-attic-ci-token";
          runtimeInputs = [
            inputs.clan-core.packages.${system}.clan-cli
            pkgs.gh
          ];
          text = ''
            : "''${MACHINE:=BIMBA}"

            token=$(clan vars get "$MACHINE" attic-ci-token/token | tr -d '\n')
            printf '%s' "$token" | gh secret set ATTIC_TOKEN
            repo=$(gh repo view --json nameWithOwner --jq .nameWithOwner)
            echo "Pushed ATTIC_TOKEN -> $repo (from $MACHINE/attic-ci-token)"
          '';
        };
      };
    };
}
