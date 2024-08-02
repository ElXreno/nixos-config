{ lib, pkgs, inputs, ... }:

{
  environment.packages = with pkgs; [
    nixd
    nixfmt-classic
    openssh
    sops

    diffutils
    findutils
    fd
    utillinux
    hostname
    man
    gnugrep
    gnupg
    gnused
    gnutar
    gzip
    xz
    zip
    unzip
    gitFull
    ripgrep
    rsync
    dua
    gawk
    which
    glibc
    (inputs.deploy-rs.defaultPackage.${builtins.currentSystem})
  ];

  environment.etcBackupExtension = ".bak";

  user.shell = "${pkgs.fish}/bin/fish";
  system.stateVersion = "24.05";

  home-manager = {
    backupFileExtension = "hm-bak";
    useGlobalPkgs = true;

    config = { pkgs, ... }: {
      home = {
        stateVersion = "24.05";
        sessionVariables = { EDITOR = "hx"; };
      };

      programs = {
        direnv = {
          enable = true;
          nix-direnv.enable = true;
        };

        helix = {
          enable = true;
          # TODO: Move to programs profile and include here
          settings = {
            editor = { completion-replace = true; };
            keys = let defaultKeys = { "C-s" = ":w"; };
            in {
              normal = defaultKeys;
              insert = defaultKeys;
            };
          };
          languages = {
            language = [
              {
                name = "nix";
                formatter.command = "nixfmt";
                language-servers = [ "nixd" ];
                auto-format = true;
              }
              {
                name = "rust";
                auto-format = true;
              }
            ];
            language-server = {
              nixd.command = "nixd";
              rust-analyzer = {
                config = {
                  diagnostics.experimental.enable = true;
                  check.features = "all";
                };
              };
            };
          };
        };

        fish.enable = true;
        starship.enable = true;
      };
    };
  };

  nix = {
    extraOptions = ''
      experimental-features = nix-command flakes
    '';

    registry.nixpkgs.flake = inputs.nixpkgs;

    nixPath = lib.mkForce [ "nixpkgs=${inputs.nixpkgs}" ];
  };

  time.timeZone = "Europe/Minsk";
}
