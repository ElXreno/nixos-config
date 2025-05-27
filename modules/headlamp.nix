{ pkgs, ... }:
pkgs.appimageTools.wrapType2 rec {
  pname = "headlamp";
  version = "0.30.0";

  src = builtins.fetchurl {
    url = "https://github.com/kubernetes-sigs/headlamp/releases/download/v${version}/Headlamp-${version}-linux-x64.AppImage";
    name = "${pname}-${version}.AppImage";
    sha256 = "sha256-S6e0/fUjdJ2Oepv3q+rCT8VnyiwZOm60wOu28a07xHA=";
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
