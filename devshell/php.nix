{ pkgs, ... }:
let
  phpMajorVer = toString 82;
in
pkgs.mkShell {
  nativeBuildInputs = with pkgs; [
    pkgs."php${phpMajorVer}"
    jetbrains.phpstorm
  ];

  buildInputs = with pkgs; [ ];
}
