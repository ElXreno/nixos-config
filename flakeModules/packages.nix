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
    in
    {
      _module.args.pkgs = pkgs;
      clan.pkgs = pkgs;

      legacyPackages.cuda = (mkPkgs true).pkgs;

      packages = customNixpkgs.packages // {
        unlock-zfs = pkgs.writeShellApplication {
          name = "unlock-zfs";
          runtimeInputs = [
            inputs.clan-core.packages.${system}.clan-cli
            pkgs.iputils
            pkgs.openssh
          ];
          text = builtins.readFile ./unlock-zfs.sh;
        };
      };
    };
}
