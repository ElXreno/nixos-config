{ pkgs, ... }:
pkgs.mkShell {
  nativeBuildInputs = with pkgs; [ jetbrains.idea-ultimate ];

  buildInputs = with pkgs; [ ];
}
