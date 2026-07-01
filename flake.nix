{
  description = "A big collection of crappy hacks";

  inputs = {
    nixpkgs = {
      url = "github:ElXreno/nixpkgs/nixos-unstable-cust";
    };

    flake-input-patcher = {
      url = "github:ElXreno/flake-input-patcher/support-non-flake-inputs";
      inputs.nixpkgs.follows = "nixpkgs";
    };

    flake-parts = {
      url = "github:hercules-ci/flake-parts";
      inputs.nixpkgs-lib.follows = "nixpkgs";
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

    nix-on-droid = {
      url = "github:nix-community/nix-on-droid";
      inputs.nixpkgs.follows = "nixpkgs";
      inputs.home-manager.follows = "home-manager";
    };

    disko = {
      url = "github:nix-community/disko";
      inputs.nixpkgs.follows = "nixpkgs";
    };

    impermanence = {
      url = "github:nix-community/impermanence";
      inputs.nixpkgs.follows = "nixpkgs";
      inputs.home-manager.follows = "home-manager";
    };

    lanzaboote = {
      url = "github:nix-community/lanzaboote";
      inputs.nixpkgs.follows = "nixpkgs";
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

    deploy-rs = {
      url = "github:serokell/deploy-rs";
      inputs.nixpkgs.follows = "nixpkgs";
    };

    niri = {
      url = "github:sodiboo/niri-flake";
      inputs.nixpkgs.follows = "nixpkgs";
      inputs.niri-stable = {
        url = "github:niri-wm/niri/v26.04";
        flake = false;
      };
    };

    stylix = {
      url = "github:nix-community/stylix";
      inputs.nixpkgs.follows = "nixpkgs";
      inputs.flake-parts.follows = "flake-parts";
    };

    firefox-addons = {
      url = "github:petrkozorezov/firefox-addons-nix";
      inputs.nixpkgs.follows = "nixpkgs";
    };

    rk-m87-sync = {
      url = "github:elxreno/rk-m87-sync";
      inputs.nixpkgs.follows = "nixpkgs";
      inputs.flake-parts.follows = "flake-parts";
    };

    nixflix = {
      url = "github:kiriwalawren/nixflix";
      inputs.nixpkgs.follows = "nixpkgs";
      inputs.treefmt-nix.follows = "treefmt-nix";
    };

    nix-minecraft = {
      url = "github:ElXreno/nix-minecraft/feat/fetchCurseForgeModpack";
      inputs.nixpkgs.follows = "nixpkgs";
    };

    proton-cachyos = {
      url = "github:powerofthe69/proton-cachyos-nix";
      inputs.nixpkgs.follows = "nixpkgs";
    };

    noctalia = {
      url = "github:noctalia-dev/noctalia-shell/legacy-v4";
      inputs.nixpkgs.follows = "nixpkgs";
    };

    noctalia-plugins = {
      url = "github:noctalia-dev/noctalia-plugins";
      flake = false;
    };

    llm-agents = {
      url = "github:numtide/llm-agents.nix";
      inputs.nixpkgs.follows = "nixpkgs";
    };

    caveman = {
      url = "github:JuliusBrussee/caveman";
      flake = false;
    };

    terranix = {
      url = "github:terranix/terranix";
      inputs.nixpkgs.follows = "nixpkgs";
      inputs.flake-parts.follows = "flake-parts";
    };

    git-hooks-nix = {
      url = "github:cachix/git-hooks.nix";
      inputs.nixpkgs.follows = "nixpkgs";
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
      inputs = patcher.patch unpatchedInputs {
        niri = {
          patches = [
            # Drop once sodiboo/niri-flake bumps niri-stable past v25.11
            # and removes the replace-service-with-usr-bin parameter.
            ./patches/niri-flake-stable-v26.patch

            ./patches/niri-flake-output-bpc.patch
          ];

          inputs.niri-stable.patches = [
            ./patches/niri-stable-bpc-pr3158.patch

            # Adds read-only `niri msg pointer` (global cursor, like hyprctl cursorpos)
            # so linux-wallpaperengine can drive mouse interaction. Upstream refuses
            # this (niri-wm/niri#1924); picked from bczhc/niri@6e2b79f.
            ./patches/niri-stable-msg-pointer.patch
          ];
        };

        noctalia.patches = [ ./patches/noctalia-no-workspace-wave.patch ];
      };
    in
    flake-parts.lib.mkFlake { inherit inputs; } {
      systems = [
        "x86_64-linux"
        "aarch64-linux"
      ];

      imports = [
        inputs.clan-core.flakeModules.default
        inputs.terranix.flakeModule
        ./flakeModules
      ];
    };
}
