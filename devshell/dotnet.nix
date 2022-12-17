{ pkgs, ... }:
let
  dotnetMajorVer = toString 6;

  dotnetSdkPackage = pkgs."dotnet-sdk_${dotnetMajorVer}";
  monoPackage = pkgs."mono${dotnetMajorVer}";
in
pkgs.mkShell {
  nativeBuildInputs = with pkgs; [
    jetbrains.rider
  ];

  buildInputs = with pkgs; [
    dotnetSdkPackage
    monoPackage
  ];

  shellHook = ''
    export PATH="$PATH:~/.dotnet/tools"
    export DOTNET_ROOT="${dotnetSdkPackage}"
  '';
}
