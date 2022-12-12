{ pkgs, ... }:
pkgs.mkShell {
  nativeBuildInputs = with pkgs; [
    androidStudioPackages.canary
  ];

  buildInputs = with pkgs; [ ];
}
