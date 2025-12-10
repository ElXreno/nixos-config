{ inputs, pkgs, ... }: inputs.hy3.packages.${pkgs.stdenv.hostPlatform.system}.hy3
