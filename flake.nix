{
  description = "A big collection of crappy hacks";

  inputs = {
    nixpkgs.url = "github:ElXreno/nixpkgs/nixos-unstable-cust";
    # nixpkgs.url = "github:NixOS/nixpkgs/nixos-unstable";

    nixos-hardware.url = "github:nixos/nixos-hardware";

    nix-index-database.url = "github:Mic92/nix-index-database";
    nix-index-database.inputs.nixpkgs.follows = "nixpkgs";

    nix-serve-ng.url = github:aristanetworks/nix-serve-ng;
    nix-serve-ng.inputs.nixpkgs.follows = "nixpkgs";

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
  };

  outputs = { self, nixpkgs, deploy-rs, ... }@inputs:
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
          Nixis-Server = {
            hostname = "nixis";
            profiles.system = {
              path = deploy-rs.lib.x86_64-linux.activate.nixos
                self.nixosConfigurations.Nixis-Server;
            };
          };
          Noxer-Server = {
            hostname = "noxer";
            profiles.system = {
              path = deploy-rs.lib.x86_64-linux.activate.nixos
                self.nixosConfigurations.Noxer-Server;
            };
          };
        };
      };
    };
}
