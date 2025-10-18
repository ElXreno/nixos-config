{
  stdenv,
  lib,
  requireFile,
  dpkg,
}:

stdenv.mkDerivation rec {
  pname = "yandex-browser-customisation";
  version = "25.8.4.822-1";

  src = requireFile {
    name = "${pname}.deb";
    hash = "sha256-Hb3cIpEAJ8GD2+E8601hly6XGvon0a0lx8qkKvzZft0=";
    url = "https://browser.yandex.ru";
  };

  nativeBuildInputs = [
    dpkg
  ];

  installPhase = ''
    mkdir $out
    cp -r var $out
  '';

  meta = with lib; {
    description = "Yandex Web Browser Customisation";
    homepage = "https://browser.yandex.ru/";
    license = licenses.unfree;
    sourceProvenance = with sourceTypes; [ binaryNativeCode ];
    maintainers = with maintainers; [
      elxreno
    ];
    platforms = [ "x86_64-linux" ];
  };
}
