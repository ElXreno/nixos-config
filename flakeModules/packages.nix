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
      pkgs = customNixpkgs.pkgs;
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
      };
    };
}
