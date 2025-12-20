{ inputs, pkgs, ... }: inputs.disko.packages.${pkgs.stdenv.hostPlatform.system}.disko
