{
  description = "A big collection of crappy hacks";

  inputs = {
    nixpkgs = {
      url = "github:ElXreno/nixpkgs/nixos-unstable-cust";
    };

    flake-input-patcher = {
      url = "github:jfly/flake-input-patcher";
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

    nixos-generators = {
      url = "github:nix-community/nixos-generators";
      inputs.nixpkgs.follows = "nixpkgs";
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
      inputs.niri-stable.url = "github:niri-wm/niri/v26.04";
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

    llm-agents = {
      url = "github:numtide/llm-agents.nix";
      inputs.nixpkgs.follows = "nixpkgs";
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
        firefox-addons.patches = [
          (patcher.fetchpatch {
            name = "fix-eval-after-license-refactor";
            # https://github.com/petrkozorezov/firefox-addons-nix/pull/2
            url = "https://github.com/xddxdd/firefox-addons-nix/commit/48b574572e98f60cd52c28b1a0161f827c3be1ae.patch";
            hash = "sha256-trqcjgn7Yn1zXiNnhGbN6yJJfM6IYX3qIFmRqGyHxCI=";
          })
        ];

        llm-agents.patches = [
          (patcher.fetchpatch {
            name = "revert-claude-code-2.1.120";
            url = "https://github.com/numtide/llm-agents.nix/commit/f630f11fa6a4881475608aee735b09dbac55aa2c.patch";
            revert = true;
            hash = "sha256-yvbcgkEiBya6MzUG2zn4Zs3CsYwrWNXI+nYbcPQ3Hn4=";
          })
        ];

        # Drop once sodiboo/niri-flake bumps niri-stable past v25.11
        # and removes the replace-service-with-usr-bin parameter.
        niri.patches = [ ./patches/niri-flake-stable-v26.patch ];
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
