{
  lib,
  stdenv,
  fetchFromGitHub,
}:

stdenv.mkDerivation {
  pname = "bios-extract";
  version = "0-unstable-2023-05-11";

  src = fetchFromGitHub {
    owner = "coreboot";
    repo = "bios_extract";
    rev = "f7e3b69870decfaab76e396a3ab7b75f0cb1a20d";
    hash = "sha256-oWTgowCx5YF3hqWELAp0iLUV6Zv2lQEGzv4FeJBcZpY=";
  };

  installPhase = ''
    install -Dm755 bios_extract $out/bin/bios_extract
  '';

  meta = {
    description = "Extract modules from AMI, Award, and Phoenix BIOS images";
    homepage = "https://github.com/coreboot/bios_extract";
    license = lib.licenses.gpl2;
    platforms = lib.platforms.linux;
    mainProgram = "bios_extract";
  };
}
