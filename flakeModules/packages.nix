{ inputs, clanMeta, ... }:
{
  perSystem =
    { system, ... }:
    let
      mkPkgs =
        cudaSupport:
        (import ../nixpkgs.nix {
          inherit inputs cudaSupport;
          namespace = clanMeta.name;
        })
          system;
      customNixpkgs = mkPkgs false;
      inherit (customNixpkgs) pkgs;
      clan-cli = inputs.clan-core.packages.${system}.clan-cli.override { nix = pkgs.nix-optimized; };
    in
    {
      _module.args.pkgs = pkgs;
      clan.pkgs = pkgs;

      legacyPackages.cuda = (mkPkgs true).pkgs;

      packages = customNixpkgs.packages // {
        inherit clan-cli;
        update-packages = pkgs.writeShellApplication {
          name = "update-packages";
          runtimeInputs = [
            pkgs.gitMinimal
            pkgs.jq
            pkgs.nix-update
          ];
          runtimeEnv = {
            UPDATE_SYSTEM = system;
            UPDATE_SELECT = ./_updatable.nix;
          };
          text = builtins.readFile ./update-packages.sh;
        };
        unlock-zfs = pkgs.writeShellApplication {
          name = "unlock-zfs";
          runtimeInputs = [
            clan-cli
            pkgs.iputils
            pkgs.openssh
          ];
          text = builtins.readFile ./unlock-zfs.sh;
        };
      };
    };
}
