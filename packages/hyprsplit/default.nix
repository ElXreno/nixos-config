{ inputs, pkgs, ... }: inputs.hyprsplit.packages.${pkgs.stdenv.hostPlatform.system}.hyprsplit
