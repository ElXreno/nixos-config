{ inputs, ... }:
{
  flake.nixOnDroidConfigurations.default = inputs.nix-on-droid.lib.nixOnDroidConfiguration {
    modules = [ ../nixOnDroid ];
    extraSpecialArgs = { inherit inputs; };
    pkgs = import inputs.nixpkgs {
      system = "aarch64-linux";
      overlays = [ inputs.nix-on-droid.overlays.default ];
    };
  };
}
