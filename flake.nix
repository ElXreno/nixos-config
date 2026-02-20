{
  description = "A big collection of crappy hacks";

  inputs = {
    nixpkgs.url = "github:ElXreno/nixpkgs/nixos-unstable-cust";

    flake-utils-plus.url = "github:ElXreno/flake-utils-plus";
    snowfall-lib = {
      # Upstream: https://github.com/snowfallorg/lib
      url = "github:ElXreno/snowfallorg-lib";
      inputs.nixpkgs.follows = "nixpkgs";
      inputs.flake-utils-plus.follows = "flake-utils-plus";
    };

    nixos-facter-modules.url = "github:nix-community/nixos-facter-modules";

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

    impermanence = {
      url = "github:nix-community/impermanence";
      inputs.nixpkgs.follows = "nixpkgs";
    };

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

    niri = {
      url = "github:sodiboo/niri-flake";
      inputs.nixpkgs.follows = "nixpkgs";
    };

    stylix = {
      url = "github:nix-community/stylix";
      inputs.nixpkgs.follows = "nixpkgs";
    };

    firefox-addons = {
      url = "github:petrkozorezov/firefox-addons-nix";
      inputs.nixpkgs.follows = "nixpkgs";
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

      overlays = with inputs; [ firefox-addons.overlays.default ];

      channels-config = {
        allowUnfree = true;
      };

      channels =
        let
          cuda-config = {
            allowUnfree = true;
            cudaSupport = true;
            nvidia.acceptLicense = true;
          };

          mkLocalZnverSystem = isa: {
            system = "x86_64-linux";
            gcc.arch = "znver${toString isa}";
            gcc.tune = "znver${toString isa}";
          };
        in
        {
          nixpkgs-znver4 = {
            input = inputs.nixpkgs;
            localSystem = mkLocalZnverSystem 4;
          };

          nixpkgs-cuda = {
            input = inputs.nixpkgs;
            config = cuda-config;
          };

          nixpkgs-cuda-znver4 = {
            input = inputs.nixpkgs;
            config = cuda-config;
            localSystem = mkLocalZnverSystem 4;
          };
        };

      homes.modules = with inputs; [
        nix-index-database.homeModules.nix-index
        plasma-manager.homeModules.plasma-manager
        stylix.homeModules.default
        niri.homeModules.stylix
      ];

      systems = {
        modules = {
          nixos = with inputs; [
            home-manager.nixosModules.home-manager
            disko.nixosModules.disko
            sops-nix.nixosModules.sops
            impermanence.nixosModules.impermanence
            niri.nixosModules.niri
            lanzaboote.nixosModules.lanzaboote
          ];
        };

        hosts = {
          AMD-Desktop.channelName = "nixpkgs-cuda";
          BIMBA.channelName = "nixpkgs-znver4";
          KURWA.channelName = "nixpkgs-cuda-znver4";
        };
      };

      outputs-builder = channels: { formatter = channels.nixpkgs.nixfmt; };
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
            interactiveSudo = true;
          };
          BIMBA = {
            hostname = "bimba";
            profiles.system = {
              path = inputs.deploy-rs.lib.x86_64-linux.activate.nixos inputs.self.nixosConfigurations.BIMBA;
            };
            activationTimeout = 1200; # For minecraft server
            confirmTimeout = 60;
          };
          DESTROYER = {
            hostname = "destroyer";
            profiles.system = {
              path = inputs.deploy-rs.lib.x86_64-linux.activate.nixos inputs.self.nixosConfigurations.DESTROYER;
            };
          };
          GRATE = {
            hostname = "grate";
            profiles.system = {
              path = inputs.deploy-rs.lib.x86_64-linux.activate.nixos inputs.self.nixosConfigurations.GRATE;
            };
          };
          INFINITY = {
            hostname = "infinity";
            profiles.system = {
              path = inputs.deploy-rs.lib.x86_64-linux.activate.nixos inputs.self.nixosConfigurations.INFINITY;
            };
            interactiveSudo = true;
          };
        };
      };

      hydraJobs =
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
    };
}
