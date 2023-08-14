{ lib, stdenv, fetchurl, makeWrapper, copyDesktopItems, makeDesktopItem, jdk11, unzip }:

stdenv.mkDerivation rec {
  pname = "lsfusion-client";
  baseVersion = "5.1";
  version = "${baseVersion}-20230731.103528-128";

  # Builds available here:
  # https://repo.lsfusion.org/nexus/service/rest/repository/browse/public/lsfusion/platform/desktop-client
  src = fetchurl {
    url = "https://repo.lsfusion.org/nexus/repository/public/lsfusion/platform/desktop-client/${baseVersion}-SNAPSHOT/desktop-client-${version}-assembly.jar";
    sha256 = "vDXuWGsRi6E/7wtin9+iLHKaHvtqpwcfUi8RMqLYd14=";
  };

  nativeBuildInputs = [ copyDesktopItems makeWrapper unzip ];

  dontUnpack = true;

  desktopItems = [
    (makeDesktopItem {
      name = pname;
      exec = pname;
      icon = pname;
      desktopName = "lsFusion Client";
    })
  ];

  postInstall = ''
    makeWrapper ${jdk11}/bin/java $out/bin/${pname} --add-flags "-jar $src"

    # Extract icon from jar binary and install
    unzip -j $src images/logo/icon_256.png

    install -Dm444 -T icon_256.png $out/share/icons/hicolor/256x256/apps/${pname}.png
  '';

  meta = with lib; {
    description = "Extremely declarative open-source language-based platform for information systems development";
    homepage = "https://lsfusion.org/";
    sourceProvenance = with sourceTypes; [ binaryBytecode ];
    license = licenses.lgpl3Only;
    maintainers = [ ];
  };
}
