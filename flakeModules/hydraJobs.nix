{ inputs, ... }:
{
  flake.hydraJobs =
    with inputs.nixpkgs.lib;

    let
      genMeta =
        drv: isNixOnDroid:
        (drv.meta or { })
        // {
          arch = removeSuffix "-linux" drv.system;
          additionalBuildArgs = if isNixOnDroid then "--impure" else "";
        };

      nixosJobs = mapAttrs (
        _name: cfg:
        let
          drv = cfg.config.system.build.toplevel;
        in
        drv
        // {
          meta = genMeta drv false;
        }
      ) inputs.self.nixosConfigurations;

      nixOnDroidJobs = concatMapAttrs (
        name: cfg:
        let
          jobName = "nix-on-droid-${name}";
          drv = cfg.activationPackage;
        in
        {
          ${jobName} = drv // {
            meta = genMeta drv true;
          };
        }
      ) inputs.self.nixOnDroidConfigurations;
    in
    nixosJobs // nixOnDroidJobs;

  flake.ciMatrix =
    with inputs.nixpkgs.lib;

    let
      nixosJobs = mapAttrs (name: cfg: {
        inherit name;
        arch = removeSuffix "-linux" cfg.config.nixpkgs.hostPlatform.system;
        additionalBuildArgs = "";
      }) inputs.self.nixosConfigurations;

      nixOnDroidJobs = concatMapAttrs (
        name: cfg:
        let
          jobName = "nix-on-droid-${name}";
        in
        {
          ${jobName} = {
            name = jobName;
            arch = removeSuffix "-linux" cfg.pkgs.stdenv.hostPlatform.system;
            additionalBuildArgs = "--impure";
          };
        }
      ) inputs.self.nixOnDroidConfigurations;
    in
    nixosJobs // nixOnDroidJobs;
}
