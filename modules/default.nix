{
  lib,
  inputs,
  namespace,
  virtual,
  ...
}:
let
  # TODO: Replace with
  # lib.filesystem.listFilesRecursive ./. |> builtins.filter (lib.hasSuffix ".nix");
  # when experimental feature `pipe-operators` will be enabled
  collectModules =
    dir: builtins.filter (lib.hasSuffix ".nix") (lib.filesystem.listFilesRecursive dir);

  nixosModules = collectModules ./nixos;
  nixosAuxiliaryModules = with inputs; [
    home-manager.nixosModules.home-manager
    impermanence.nixosModules.impermanence
    lanzaboote.nixosModules.lanzaboote
    rk-m87-sync.nixosModules.default
    nixflix.nixosModules.default

    niri.nixosModules.niri
    {
      niri-flake.cache.enable = false;
    }
  ];

  homeModules = collectModules ./home;
  homeAuxiliaryModules = with inputs; [
    nix-index-database.homeModules.nix-index
    plasma-manager.homeModules.plasma-manager
    stylix.homeModules.default
    niri.homeModules.stylix
    noctalia.homeModules.default
  ];

in
{
  imports = nixosModules ++ nixosAuxiliaryModules;

  home-manager = {
    useGlobalPkgs = true;
    useUserPackages = true;
    backupFileExtension = ".bak";

    extraSpecialArgs = {
      inherit namespace inputs virtual;
    };

    sharedModules = homeModules ++ homeAuxiliaryModules;
  };
}
