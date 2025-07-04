{
  description = "A big collection of crappy hacks";

  inputs = {
    nixpkgs.url = "github:ElXreno/nixpkgs/nixos-unstable-cust";

    nixos-hardware.url = "github:nixos/nixos-hardware";
    nixos-facter-modules.url = "github:numtide/nixos-facter-modules";

    nix-index-database.url = "github:nix-community/nix-index-database";
    nix-index-database.inputs.nixpkgs.follows = "nixpkgs";

    nix-on-droid = {
      url = "github:nix-community/nix-on-droid";
      inputs.nixpkgs.follows = "nixpkgs";
      inputs.home-manager.follows = "home-manager";
    };

    disko.url = "github:nix-community/disko";
    disko.inputs.nixpkgs.follows = "nixpkgs";

    lanzaboote.url = "github:nix-community/lanzaboote";
    lanzaboote.inputs.nixpkgs.follows = "nixpkgs";

    # home-manager stuff
    home-manager.url = "github:nix-community/home-manager";
    home-manager.inputs.nixpkgs.follows = "nixpkgs";

    plasma-manager = {
      url = "github:nix-community/plasma-manager";
      inputs.nixpkgs.follows = "nixpkgs";
      inputs.home-manager.follows = "home-manager";
    };

    # deploy-rs stuff
    deploy-rs.url = "github:serokell/deploy-rs";
    deploy-rs.inputs.nixpkgs.follows = "nixpkgs";

    sops-nix = {
      url = "github:Mic92/sops-nix";
      inputs.nixpkgs.follows = "nixpkgs";
    };
  };

  outputs =
    {
      self,
      nixpkgs,
      deploy-rs,
      nix-on-droid,
      ...
    }@inputs:
    let
      lib = nixpkgs.lib;
      lib' = import ./lib { inherit lib; };
    in
    {
      nixosModules = lib'.rakeLeaves ./modules;

      nixosProfiles = lib'.rakeLeaves ./profiles;

      nixosRoles = import ./roles;

      nixOnDroidConfigurations.default = nix-on-droid.lib.nixOnDroidConfiguration {
        modules = [ ./nix-on-droid ];
        extraSpecialArgs = { inherit inputs; };

        pkgs = import nixpkgs {
          system = "aarch64-linux";

          overlays = [ nix-on-droid.overlays.default ];
        };
      };

      nixosConfigurations =
        with lib;
        let
          hosts = builtins.attrNames (builtins.readDir ./machines);
          mkHost =
            name:
            nixosSystem {
              system = builtins.readFile (./machines + "/${name}/system");
              modules = [
                (import (./machines + "/${name}"))
                { device = name; }
              ];
              specialArgs = { inherit inputs lib'; };
            };
        in
        genAttrs hosts mkHost;

      diskoConfigurations = lib'.rakeLeaves ./disko;

      devShells.x86_64-linux =
        with lib;
        let
          pkgs = import nixpkgs {
            system = "x86_64-linux";
          };
          mkShells = mapAttrs (
            _name: value:
            import value {
              inherit inputs pkgs;
              system = "x86_64-linux";
            }
          );
        in
        mkShells (lib'.rakeLeaves ./devshell);

      deploy = {
        user = "root";
        sshUser = "elxreno";
        nodes = {
          AMD-Desktop = {
            hostname = "desktop";
            profiles.system = {
              path = deploy-rs.lib.x86_64-linux.activate.nixos self.nixosConfigurations.AMD-Desktop;
            };
            fastConnection = true;
          };
        };
      };

      hydraJobs =
        with lib;
        (mapAttrs (_: val: val.config.system.build.toplevel) self.nixosConfigurations)
        // (concatMapAttrs (name: val: {
          "nix-on-droid-${name}" = val.activationPackage;
        }) self.nixOnDroidConfigurations);

      ci =
        with lib;
        mapAttrsToList (name: value: {
          inherit name;
          arch = strings.removeSuffix "-linux" value.system;
          additionalBuildArgs = if (name == "nix-on-droid-default") then "--impure" else "";
        }) self.hydraJobs;
    };
}
