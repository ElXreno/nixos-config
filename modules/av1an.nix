{ lib, rustPlatform, fetchCrate, makeWrapper, nasm, pkg-config, ffmpeg
, vapoursynth, rav1e, svt-av1, libaom, libvpx, x264, x265, mkvtoolnix }:

rustPlatform.buildRustPackage rec {
  pname = "av1an";
  version = "0.4.1";

  src = fetchCrate {
    inherit pname version;
    sha256 = "sha256-mM0zXVmmj5ZVtMr8upWCOrxsYEUFs2PZX4E/4NYAZd8=";
  };

  cargoSha256 = "sha256-uzaajaBL67Jca9b9E/UHUv56dmQ2XSjZTNdhOiquUrU=";

  nativeBuildInputs = [ makeWrapper rustPlatform.bindgenHook nasm pkg-config ];
  buildInputs = [ ffmpeg vapoursynth ];

  postInstall = ''
    wrapProgram $out/bin/av1an \
      --prefix PATH : ${
        lib.makeBinPath [
          vapoursynth.python3
          rav1e
          svt-av1
          libaom
          libvpx
          x264
          x265
          mkvtoolnix
        ]
      } \
      --prefix PYTHONPATH : "${vapoursynth}/${vapoursynth.python3.sitePackages}"
  '';

  meta = with lib; {
    description = "Cross-platform command-line encoding framework";
    longDescription = ''
      Cross-platform command-line AV1 / VP9 / HEVC / H264 encoding framework
      with per scene quality encoding.
      Features: https://github.com/master-of-zen/Av1an/tree/${version}#features
    '';
    homepage = "https://github.com/master-of-zen/Av1an";
    changelog =
      "https://github.com/master-of-zen/Av1an/releases/tag/${version}";
    license = licenses.bsd2;
    maintainers = [ ];
  };
}
