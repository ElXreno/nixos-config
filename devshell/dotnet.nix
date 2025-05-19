{ pkgs, ... }:
let
  dotnetMajorVer = toString 6;

  dotnetSdkPackage = pkgs."dotnet-sdk_${dotnetMajorVer}";
  monoPackage = pkgs."mono${dotnetMajorVer}";
in
pkgs.mkShell {
  nativeBuildInputs = with pkgs; [ jetbrains.rider ];

  buildInputs = with pkgs; [
    dotnetSdkPackage
    monoPackage
  ];

  NIX_LD = pkgs.lib.fileContents "${pkgs.stdenv.cc}/nix-support/dynamic-linker";

  shellHook = ''
    export PATH="$PATH:~/.dotnet/tools"
    export DOTNET_ROOT="${dotnetSdkPackage}"
  '';
}
