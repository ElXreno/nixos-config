{ lib, rust, stdenv, rustPlatform, fetchCrate, makeWrapper, nasm, pkg-config, llvmPackages, ffmpeg, vapoursynth, rav1e }:

rustPlatform.buildRustPackage rec {
  pname = "av1an";
  version = "0.2.0";

  src = fetchCrate {
    inherit pname version;
    sha256 = "sha256-WUroqn2yTr0wwo1GduRjNFKMi3mjdsKwoMmWLcb7c4M=";
  };

  cargoSha256 = "sha256-ma5z8W8FpRphbz9jHaseFlH0iGnWm7kLbw6njzHfb5A=";

  nativeBuildInputs = [ makeWrapper nasm pkg-config llvmPackages.clang ];
  buildInputs = [ ffmpeg vapoursynth ];

  LIBCLANG_PATH = "${llvmPackages.libclang.lib}/lib";

  postInstall = ''
    wrapProgram $out/bin/av1an \
      --prefix PATH ${rav1e}/bin
  '';

  meta = with lib; {
    description = "Cross-platform command-line encoding framework";
    longDescription = ''
      Cross-platform command-line AV1 / VP9 / HEVC / H264 encoding framework
      with per scene quality encoding.
      Features: https://github.com/master-of-zen/Av1an#main-features
    '';
    homepage = "https://github.com/master-of-zen/Av1an";
    changelog = "https://github.com/master-of-zen/Av1an/releases/tag/${version}";
    license = licenses.bsd2;
    maintainers = [ ];
  };
}
