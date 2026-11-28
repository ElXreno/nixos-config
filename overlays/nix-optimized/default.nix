{ inputs, ... }:
_final: prev:
let
  inherit (prev.stdenv.hostPlatform) system;
  optimized = inputs.nix-optimized.packages.${system};
in
{
  inherit (optimized) nix-optimized;

  nix-eval-jobs-optimized =
    inputs.nix-optimized.inputs.nixpkgs.legacyPackages.${system}.nix-eval-jobs.override
      {
        nixComponents = optimized.nix-optimized-2_35.libs // {
          inherit (optimized.nix-optimized-2_35) version nix-cli;
        };
      };
}
