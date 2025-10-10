{
  description = "A big collection of crappy hacks";

  inputs = {
    nixpkgs.url = "github:ElXreno/nixpkgs/nixos-unstable-cust";
    nixpkgs-cuda.follows = "nixpkgs";

    # Upstream: https://github.com/snowfallorg/lib
    snowfall-lib = {
      url = "github:ElXreno/snowfallorg-lib";
      inputs.nixpkgs.follows = "nixpkgs";
    };

    nixos-facter-modules.url = "github:numtide/nixos-facter-modules";

    nix-index-database.url = "github:nix-community/nix-index-database";
    nix-index-database.inputs.nixpkgs.follows = "nixpkgs";

    nix-on-droid = {
      url = "github:nix-community/nix-on-droid";
      inputs.nixpkgs.follows = "nixpkgs";
      inputs.home-manager.follows = "home-manager";
    };

    nixos-generators = {
      url = "github:nix-community/nixos-generators";
      inputs.nixpkgs.follows = "nixpkgs";
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

    hyprland = {
      url = "github:hyprwm/Hyprland";
      inputs.nixpkgs.follows = "nixpkgs";
    };
    split-monitor-workspaces = {
      url = "github:Duckonaut/split-monitor-workspaces";
      inputs.hyprland.follows = "hyprland";
    };
  };

  outputs =
    inputs:
    (inputs.snowfall-lib.mkFlake {
      inherit inputs;

      src = ./.;

      snowfall = rec {
        namespace = "elxreno-nix";
        meta = {
          name = namespace;
          title = "ElXreno's NixOS Configurations";
        };
      };

      channels-config = {
        allowUnfree = true;
      };

      channels.nixpkgs-cuda.config = {
        allowUnfree = true;
        cudaSupport = true;
        nvidia.acceptLicense = true;
      };

      homes.modules = with inputs; [
        nix-index-database.homeModules.nix-index
        plasma-manager.homeModules.plasma-manager
      ];

      systems = {
        modules = {
          nixos = with inputs; [
            home-manager.nixosModules.home-manager
            disko.nixosModules.disko
            sops-nix.nixosModules.sops
          ];
        };

        hosts =
          let
            withNvidiaCfg = {
              channelName = "nixpkgs-cuda";

              specialArgs = {
                withNvidia = true;
              };
            };
          in
          {
            KURWA = withNvidiaCfg;
            AMD-Desktop = withNvidiaCfg;
          };
      };

      outputs-builder = channels: { formatter = channels.nixpkgs.nixfmt-rfc-style; };
    })
    // {
      nixOnDroidConfigurations.default = inputs.nix-on-droid.lib.nixOnDroidConfiguration {
        modules = [ ./nix-on-droid ];
        extraSpecialArgs = { inherit inputs; };

        pkgs = import inputs.nixpkgs {
          system = "aarch64-linux";

          overlays = [ inputs.nix-on-droid.overlays.default ];
        };
      };

      deploy = {
        user = "root";
        sshUser = "elxreno";
        nodes = {
          AMD-Desktop = {
            hostname = "desktop";
            profiles.system = {
              path = inputs.deploy-rs.lib.x86_64-linux.activate.nixos inputs.self.nixosConfigurations.AMD-Desktop;
            };
            fastConnection = true;
          };
          DESTROYER = {
            hostname = "destroyer";
            profiles.system = {
              path = inputs.deploy-rs.lib.x86_64-linux.activate.nixos inputs.self.nixosConfigurations.DESTROYER;
            };
            fastConnection = true;
          };
          INFINITY = {
            hostname = "infinity";
            profiles.system = {
              path = inputs.deploy-rs.lib.x86_64-linux.activate.nixos inputs.self.nixosConfigurations.INFINITY;
            };
            fastConnection = true;
            interactiveSudo = true;
          };
        };
      };

      hydraJobs =
        with inputs.nixpkgs.lib;
        (mapAttrs (_: val: val.config.system.build.toplevel) inputs.self.nixosConfigurations)
        // (concatMapAttrs (name: val: {
          "nix-on-droid-${name}" = val.activationPackage;
        }) inputs.self.nixOnDroidConfigurations);

      ci =
        with inputs.nixpkgs.lib;
        mapAttrsToList (name: value: {
          inherit name;
          arch = strings.removeSuffix "-linux" value.system;
          additionalBuildArgs = if (name == "nix-on-droid-default") then "--impure" else "";
        }) inputs.self.hydraJobs;
    };
}
