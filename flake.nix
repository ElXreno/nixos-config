{
  description = "A big collection of crappy hacks";

  inputs = {
    nixpkgs = {
      url = "github:NixOS/nixpkgs/nixos-unstable";
    };

    flake-input-patcher = {
      url = "github:jfly/flake-input-patcher";
      inputs.nixpkgs.follows = "nixpkgs";
      inputs.systems.follows = "systems";
    };

    zfs-slab-mobility = {
      url = "github:ElXreno/zfs-slab-mobility";
      inputs.nixpkgs.follows = "nixpkgs";
      inputs.flake-utils.follows = "flake-utils";
    };

    flake-parts = {
      url = "github:hercules-ci/flake-parts";
      inputs.nixpkgs-lib.follows = "nixpkgs";
    };

    systems.url = "github:nix-systems/default";

    flake-utils = {
      url = "github:numtide/flake-utils";
      inputs.systems.follows = "systems";
    };

    clan-core = {
      url = "github:ElXreno/clan-core";
      inputs.nixpkgs.follows = "nixpkgs";
      inputs.flake-parts.follows = "flake-parts";
      inputs.disko.follows = "disko";
      inputs.treefmt-nix.follows = "treefmt-nix";
    };

    nix-index-database = {
      url = "github:nix-community/nix-index-database";
      inputs.nixpkgs.follows = "nixpkgs";
    };

    disko = {
      url = "github:nix-community/disko";
      inputs.nixpkgs.follows = "nixpkgs";
    };

    gddr6 = {
      url = "github:olealgoritme/gddr6";
      inputs.nixpkgs.follows = "nixpkgs";
      inputs.utils.follows = "flake-utils";
    };

    impermanence = {
      url = "github:nix-community/impermanence";
      inputs.nixpkgs.follows = "nixpkgs";
      inputs.home-manager.follows = "home-manager";
    };

    lanzaboote = {
      url = "github:nix-community/lanzaboote";
      inputs.nixpkgs.follows = "nixpkgs";
      inputs.pre-commit.follows = "git-hooks-nix";
    };

    home-manager = {
      url = "github:nix-community/home-manager";
      inputs.nixpkgs.follows = "nixpkgs";
    };

    plasma-manager = {
      url = "github:nix-community/plasma-manager";
      inputs.nixpkgs.follows = "nixpkgs";
      inputs.home-manager.follows = "home-manager";
    };

    niri = {
      url = "github:epireyn/niri-flake";
      inputs.nixpkgs.follows = "nixpkgs";
      inputs.nixpkgs-stable.follows = "nixpkgs";
    };

    stylix = {
      url = "github:nix-community/stylix";
      inputs.nixpkgs.follows = "nixpkgs";
      inputs.flake-parts.follows = "flake-parts";
    };

    firefox-addons = {
      url = "github:petrkozorezov/firefox-addons-nix";
      inputs.nixpkgs.follows = "nixpkgs";
      inputs.flake-utils.follows = "flake-utils";
    };

    rk-m87-sync = {
      url = "github:elxreno/rk-m87-sync";
      inputs.nixpkgs.follows = "nixpkgs";
      inputs.flake-parts.follows = "flake-parts";
    };

    flydigictl = {
      url = "github:ElXreno/flydigictl";
      inputs.nixpkgs.follows = "nixpkgs";
      inputs.flake-parts.follows = "flake-parts";
      inputs.crane.follows = "lanzaboote/crane";
    };

    ryukaze = {
      url = "github:ElXreno/ryukaze/titan-18-hx";
      inputs.nixpkgs.follows = "nixpkgs";
    };

    roj = {
      url = "git+ssh://git@github.com/ElXreno/roj";
      inputs.nixpkgs.follows = "nixpkgs";
    };

    courbet = {
      url = "github:ElXreno/nixos-courbet";
      inputs.nixpkgs.follows = "nixpkgs";
    };

    nix-optimized.url = "github:ElXreno/nix-optimized";

    nix-keenetic.url = "github:ElXreno/nix-keenetic";

    nixflix = {
      url = "github:kiriwalawren/nixflix";
      inputs.nixpkgs.follows = "nixpkgs";
      inputs.treefmt-nix.follows = "treefmt-nix";
      inputs.mkdocs-catppuccin.follows = "";
      inputs.vpn-confinement.follows = "vpn-confinement";
    };

    vpn-confinement.url = "github:Maroka-chan/VPN-Confinement";

    niks3 = {
      url = "github:Mic92/niks3";
      inputs.nixpkgs.follows = "nixpkgs";
      inputs.treefmt-nix.follows = "treefmt-nix";
    };

    nixbot = {
      url = "github:Mic92/nixbot";
      inputs.nixpkgs.follows = "nixpkgs";
      inputs.treefmt-nix.follows = "treefmt-nix";
    };

    nix-minecraft = {
      url = "github:Infinidoge/nix-minecraft";
      inputs.nixpkgs.follows = "nixpkgs";
      inputs.systems.follows = "systems";
      inputs.flake-compat.follows = "";
    };

    spicetify-nix = {
      url = "github:Gerg-L/spicetify-nix";
      inputs.nixpkgs.follows = "nixpkgs";
      inputs.systems.follows = "systems";
    };

    llm-agents = {
      url = "github:numtide/llm-agents.nix/811c205ef60d81ebd4dae2b4d9263cedd2a69548";
      inputs.nixpkgs.follows = "nixpkgs";
      inputs.flake-parts.follows = "flake-parts";
      inputs.treefmt-nix.follows = "treefmt-nix";
      inputs.systems.follows = "systems";
    };

    terranix = {
      url = "github:terranix/terranix";
      inputs.nixpkgs.follows = "nixpkgs";
      inputs.flake-parts.follows = "flake-parts";
      inputs.systems.follows = "systems";
    };

    git-hooks-nix = {
      url = "github:cachix/git-hooks.nix";
      inputs.nixpkgs.follows = "nixpkgs";
      inputs.flake-compat.follows = "";
    };

    treefmt-nix = {
      url = "github:numtide/treefmt-nix";
      inputs.nixpkgs.follows = "nixpkgs";
    };
  };

  outputs =
    { flake-parts, ... }@unpatchedInputs:
    let
      system = "x86_64-linux";

      patcher = unpatchedInputs.flake-input-patcher.lib.${system};
      inputs = patcher.patch {
        inherit unpatchedInputs;
        flakePath = ./.;
        patchSpec = {
          nixpkgs.patches = [
            ./patches/nixpkgs-sony-headphones-client.patch
            ./patches/nixpkgs-plasma6-mobile.patch
            ./patches/nixpkgs-haskell-test-wrapper.patch

            ./patches/nixpkgs-zfs-linux-7.3.patch

            # NixOS/nixpkgs#578694
            (patcher.fetchpatch {
              name = "graalvm-native-image-rpath.patch";
              url = "https://github.com/NixOS/nixpkgs/commit/ec035b26756bac85f2c9255d68dc8427ab09a54a.patch";
              hash = "sha256-IF+bTSOV0VVXq/KXv6SGMqvrXEKjCkRQvBKgZ3Y/Ofs=";
            })
            (patcher.fetchpatch {
              name = "graalvm-oracle-onnxruntime.patch";
              url = "https://github.com/NixOS/nixpkgs/commit/3a878b607f3a3e68a690f64bbda8edd63808552d.patch";
              hash = "sha256-Lj3RlXOUD/qvirmVaWWK1n88WGLtgJcJQS/NyXbx+lo=";
            })

            # NixOS/nixpkgs#583684
            (patcher.fetchpatch {
              name = "nvidia-production-620.70.06.patch";
              url = "https://github.com/NixOS/nixpkgs/commit/1fc17581c358927eae6449a131ad6191b7e30233.patch";
              hash = "sha256-M434gFCP5fEq12dRsQSdsfT9ymwVJFA1ti5lhXeUZd4=";
            })
          ];

          nix-minecraft.patches = [
            ./patches/nix-minecraft-fetch-curseforge-modpack.patch
          ];

          niri = {
            patches = [
              ./patches/niri-flake-output-max-bpc.patch
              ./patches/niri-flake-output-srgb-clamp.patch
              ./patches/niri-flake-output-dithering.patch
            ];

            inputs.niri-unstable.patches = [
              ./patches/niri-unstable-srgb-clamp.patch

              # niri-wm/niri#2201
              ./patches/niri-lock-backdrop.patch

              # niri-wm/niri#3800
              ./patches/niri-virtual-outputs.patch

              # Turns the laptop panel off on lid close when only a virtual output remains.
              ./patches/niri-virtual-output-lid.patch

              # Output `dithering` and `max-bpc` through the NVIDIA connector properties.
              ./patches/niri-output-dithering.patch
            ];
          };

          # workaround for hooks installed into whatever repo the devShell is entered from
          git-hooks-nix.patches = [
            ./patches/git-hooks-install-marker.patch
          ];
        };
      };
      parts =
        flake-parts.lib.mkFlake
          {
            inherit inputs;
            specialArgs = { inherit unpatchedInputs; };
          }
          {
            systems = [ "x86_64-linux" ];

            imports = [
              unpatchedInputs.clan-core.flakeModules.default
              unpatchedInputs.terranix.flakeModule
              ./flakeModules
            ];
          };
    in
    unpatchedInputs.nixpkgs.lib.genAttrs (import ./flakeModules/_outputs.nix) (name: parts.${name})
    // {
      herculesCI = import ./herculesCI.nix { inherit unpatchedInputs; };
    };
}
