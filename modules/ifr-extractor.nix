{ fetchFromGitHub, lib, stdenv, cmake }:

stdenv.mkDerivation rec {
  pname = "ifr-extractor";
  version = "0.3.7";

  src = fetchFromGitHub {
    owner = "LongSoft";
    repo = "Universal-IFR-Extractor";
    rev = version;
    sha256 = "sha256-vaF8mWXOjqejnNgTm5W6wbg+YoE99U3hCH5a5yClgrE=";
  };

  nativeBuildInputs = [ cmake ];

  installPhase = ''
    install -Dm 0755 ifrextract $out/bin/ifrextract
  '';

  meta = with lib; {
    description =
      "Utility that can extract the internal forms represenation from both EFI and UEFI modules";
    longDescription = ''
      Utility to extract the internal forms representation from both EFI
      and UEFI drivers/applications into human readable text file.
    '';
    license = licenses.gpl3Only;
    maintainers = with maintainers;
      [ ]; # TODO: add myself to the maintainers list
    platforms = platforms.all;
  };
}
