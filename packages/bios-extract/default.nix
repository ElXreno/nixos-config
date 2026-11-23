{
  lib,
  stdenv,
  fetchFromGitHub,
  nix-update-script,
}:

stdenv.mkDerivation {
  pname = "bios-extract";
  version = "0-unstable-2023-06-10";

  src = fetchFromGitHub {
    owner = "coreboot";
    repo = "bios_extract";
    rev = "0a7bc1d71735ef97b00dfec0fd54a02fcc5d1bb0";
    hash = "sha256-gmQu8a/RHIRMZuWKLAYjUiQ2KTtu3uOf63g31k+jTGQ=";
  };

  installPhase = ''
    install -Dm755 bios_extract $out/bin/bios_extract
  '';

  passthru.updateScript = nix-update-script { extraArgs = [ "--version=branch" ]; };

  meta = {
    description = "Extract modules from AMI, Award, and Phoenix BIOS images";
    homepage = "https://github.com/coreboot/bios_extract";
    license = lib.licenses.gpl2;
    platforms = lib.platforms.linux;
    mainProgram = "bios_extract";
  };
}
