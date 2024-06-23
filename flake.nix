{
  description = "A big collection of crappy hacks";

  inputs = {
    nixpkgs.url = "github:ElXreno/nixpkgs/nixos-unstable-cust";
    # nixpkgs.url = "github:NixOS/nixpkgs/nixos-unstable";

    nixos-hardware.url = "github:nixos/nixos-hardware";

    nix-index-database.url = "github:Mic92/nix-index-database";
    nix-index-database.inputs.nixpkgs.follows = "nixpkgs";

    nix-on-droid = {
      url = "github:t184256/nix-on-droid";
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
      url = "github:pjones/plasma-manager";
      inputs.nixpkgs.follows = "nixpkgs";
      inputs.home-manager.follows = "home-manager";
    };

    # impermanence
    impermanence.url = "github:nix-community/impermanence";

    # deploy-rs stuff
    deploy-rs.url = "github:serokell/deploy-rs";
    deploy-rs.inputs.nixpkgs.follows = "nixpkgs";

    sops-nix = {
      url = "github:Mic92/sops-nix";
      inputs.nixpkgs.follows = "nixpkgs";
    };

    rust-overlay.url = "github:oxalica/rust-overlay";
    rust-overlay.inputs.nixpkgs.follows = "nixpkgs";

    nur-xddxdd = {
      url = "github:xddxdd/nur-packages";
      inputs.nixpkgs.follows = "nixpkgs";
    };

    teledump.url = "github:ElXreno/teledump";
    simple-reply-bot.url = "github:ElXreno/simple-reply-bot";

    elfshaker.url = "github:elfshaker/elfshaker";
    elfshaker.inputs.nixpkgs.follows = "nixpkgs";
  };

  outputs = { self, nixpkgs, disko, deploy-rs, nix-on-droid, ... }@inputs:
    let
      findModules = dir:
        builtins.concatLists (builtins.attrValues (builtins.mapAttrs
          (name: type:
            if type == "regular" then
              [{
                name = builtins.elemAt (builtins.match "(.*)\\.nix" name) 0;
                value = dir + "/${name}";
              }]
            else if (builtins.readDir (dir + "/${name}"))
              ? "default.nix" then [{
              inherit name;
              value = dir + "/${name}";
            }] else
              findModules (dir + "/${name}"))
          (builtins.readDir dir)));
    in
    {
      nixosModules = builtins.listToAttrs (findModules ./modules);

      nixosProfiles = builtins.listToAttrs (findModules ./profiles);

      nixosRoles = import ./roles;

      nixOnDroidConfigurations.default = nix-on-droid.lib.nixOnDroidConfiguration {
        modules = [ ./nix-on-droid ];
        extraSpecialArgs = { inherit inputs; };
      };

      nixosConfigurations = with nixpkgs.lib;
        let
          hosts = builtins.attrNames (builtins.readDir ./machines);
          mkHost = name:
            nixosSystem {
              system = builtins.readFile (./machines + "/${name}/system");
              modules =
                [ (import (./machines + "/${name}")) { device = name; } ];
              specialArgs = { inherit inputs; };
            };
        in
        genAttrs hosts mkHost;

      diskoConfigurations = builtins.listToAttrs (findModules ./disko);

      legacyPackages.x86_64-linux =
        (builtins.head (builtins.attrValues self.nixosConfigurations)).pkgs;

      inherit (deploy-rs) defaultApp;

      devShells.x86_64-linux = with nixpkgs.lib;
        let
          inherit ((builtins.head (builtins.attrValues self.nixosConfigurations))) pkgs config;
          mkShells = mapAttrs
            (_name: value: import value { inherit pkgs config; });
        in
        mkShells (builtins.listToAttrs (findModules ./devshell));

      deploy = {
        user = "root";
        sshUser = "elxreno";
        nodes = {
          AMD-Desktop = {
            hostname = "desktop";
            profiles.system = {
              path = deploy-rs.lib.x86_64-linux.activate.nixos
                self.nixosConfigurations.AMD-Desktop;
            };
            fastConnection = true;
          };
          romeo = {
            hostname = "100.69.61.18";
            profiles.system = {
              path = deploy-rs.lib.x86_64-linux.activate.nixos
                self.nixosConfigurations.romeo;
            };
            remoteBuild = true;
          };
        };
      };

      hydraJobs = with nixpkgs.lib;
        (mapAttrs (_: val: val.config.system.build.toplevel)
          self.nixosConfigurations
        );
    };
}
