{
  stdenv,
  lib,
  requireFile,
  dpkg,
}:

stdenv.mkDerivation rec {
  pname = "yandex-browser-customisation";
  version = "0.2512.2817.0504";

  src = requireFile {
    name = "${pname}.deb";
    hash = "sha256-XalvPw+Dt6Y9dBbMBqvP++QjfLWMR1CcM3Bv2KcZd+w=";
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
