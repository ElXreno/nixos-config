{
  lib,
  stdenv,
  fetchFromGitHub,
  makeWrapper,
  unixtools,
  libelf
}:

stdenv.mkDerivation rec {
  pname = "esp2elf";
  rev = "87ac9e2a8720bd2f1b43e3e683dd4038a9769071";
  shortRev = builtins.substring 0 7 rev;
  version = "git-${shortRev}";

  src = fetchFromGitHub {
    inherit rev;
    owner = "raburton";
    repo = pname;
    hash = "sha256-5F0GcY255ejKEoOgoXS1DN3fMj/jC6oug6vi8LdSHyU=";
  };

  nativeBuildInputs = [
    makeWrapper
    unixtools.xxd
    libelf
  ];

  buildInputs = [ ];

  installPhase = ''
    install -Dm 0755 esp2elf $out/bin/esp2elf
  '';

  meta = with lib; {
    description = "Convert esp8266 rom to elf file, for easier analysis";
    mainProgram = "esp2elf";
    homepage = "https://github.com/raburton/esp2elf";
    license = licenses.unfree; # No license file, so unfree
    platforms = platforms.unix;
    maintainers = with maintainers; [
      elxreno
    ];
  };
}
