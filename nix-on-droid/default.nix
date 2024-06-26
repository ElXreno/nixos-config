{ lib, pkgs, inputs, ... }:

{
  environment.packages = with pkgs; [
    helix
    nil
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
        sessionVariables = {
          EDITOR = "hx";
        };
      };

      programs = {
        direnv = {
          enable = true;
          nix-direnv.enable = true;
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
