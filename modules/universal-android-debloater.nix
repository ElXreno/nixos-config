{ rustPlatform
, fetchFromGitHub
, lib
, makeWrapper
, rust-bin
, lld
, android-tools
, xorg
}:

rustPlatform.buildRustPackage rec {
  pname = "universal-android-debloater";
  version = "0.5.1";

  src = fetchFromGitHub {
    owner = "0x192";
    repo = pname;
    rev = version;
    sha256 = "sha256-0ByB/jRJze9B2o4S1nevy+uIvEpjMszAVB0pB6WWocU=";
  };

  cargoSha256 = "sha256-ko8+mutR4DuFQqgWY6/F4s8j9VGuwSTLntCm/nJ2Goo=";

  nativeBuildInputs = [
    makeWrapper
    rust-bin.nightly.latest.minimal
    # Cringelous hack to get working derivation.
    lld
  ];

  postInstall = ''
    wrapProgram $out/bin/uad_gui \
      --prefix PATH : "${lib.makeBinPath [ android-tools ]}" \
      --prefix LD_LIBRARY_PATH : "${lib.makeLibraryPath (with xorg; [ libX11 libXcursor libXrandr libXi ])}"
  '';

  buildNoDefaultFeatures = true;
  buildFeatures = [ "wgpu" "no-self-update" ];

  doCheck = false;

  meta = with lib; {
    description = "UAD is a tool for debloating Android phones.";
    homepage = "https://github.com/0x192/universal-android-debloater";
    license = licenses.gpl3;
    maintainers = [ maintainers.elxreno ];
  };
}
