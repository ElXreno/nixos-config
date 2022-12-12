{ pkgs, ... }:
pkgs.mkShell {
  nativeBuildInputs = with pkgs; [
    jetbrains.rider
  ];

  buildInputs = with pkgs; [
    dotnet-sdk_6
    mono6
  ];
}
