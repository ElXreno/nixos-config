{ pkgs, fetchurl, ... }:
pkgs.appimageTools.wrapType2 rec {
  pname = "headlamp";
  version = "0.31.1";

  src = fetchurl {
    url = "https://github.com/kubernetes-sigs/headlamp/releases/download/v${version}/Headlamp-${version}-linux-x64.AppImage";
    name = "${pname}-${version}.AppImage";
    hash = "sha256-Ze6c5IImWbPyBivwavy7IRrVD40HLr8UAJb69uMKu/A=";
  };

  extraInstallCommands =
    let
      contents = pkgs.appimageTools.extractType1 { inherit pname src version; };
    in
    ''
      mkdir -p "$out/share/applications"
      mkdir -p "$out/share/lib/headlamp"
      cp -r ${contents}/* "$out/share/lib/headlamp"
      cp "${contents}/${pname}.desktop" "$out/share/applications/"
      substituteInPlace $out/share/applications/${pname}.desktop --replace 'Exec=AppRun' 'Exec=${pname}'
    '';
}
