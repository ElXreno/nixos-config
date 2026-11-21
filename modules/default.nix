{
  lib,
  inputs,
  namespace,
  ...
}:
let
  collectModules =
    dir:
    builtins.filter (p: lib.hasSuffix ".nix" p && !lib.hasPrefix "_" (baseNameOf p)) (
      lib.filesystem.listFilesRecursive dir
    );

  nixosModules = collectModules ./nixos;
  nixosAuxiliaryModules = with inputs; [
    home-manager.nixosModules.home-manager
    impermanence.nixosModules.impermanence
    lanzaboote.nixosModules.lanzaboote
    rk-m87-sync.nixosModules.default
    flydigictl.nixosModules.default
    nixflix.nixosModules.default
    nix-minecraft.nixosModules.minecraft-servers
    niks3.nixosModules.niks3
    niks3.nixosModules.niks3-auto-upload

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
    flydigictl.homeModules.default
    spicetify-nix.homeManagerModules.spicetify
  ];

in
{
  imports = nixosModules ++ nixosAuxiliaryModules;
  disabledModules = [ inputs.vpn-confinement.nixosModules.default ];

  home-manager = {
    useGlobalPkgs = true;
    useUserPackages = true;
    backupFileExtension = "bak";

    extraSpecialArgs = {
      inherit namespace inputs;
    };

    sharedModules = homeModules ++ homeAuxiliaryModules;
  };
}
