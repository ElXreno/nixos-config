{ pkgs, ... }:
pkgs.mkShell {
  nativeBuildInputs = with pkgs; [ androidStudioPackages.canary flutter ];

  buildInputs = with pkgs; [ ];
}
