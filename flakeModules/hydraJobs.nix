{ inputs, ... }:
{
  flake.hydraJobs = inputs.nixpkgs.lib.mapAttrs (
    _name: cfg: cfg.config.system.build.toplevel
  ) inputs.self.nixosConfigurations;

  flake.ciMatrix =
    with inputs.nixpkgs.lib;

    mapAttrs (name: cfg: {
      inherit name;
      arch = removeSuffix "-linux" cfg.pkgs.stdenv.hostPlatform.system;
      additionalBuildArgs = "";
    }) inputs.self.nixosConfigurations;
}
