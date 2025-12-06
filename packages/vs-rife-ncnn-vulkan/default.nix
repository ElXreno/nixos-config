{
  lib,
  stdenv,
  fetchFromGitHub,
  cmake,
  meson,
  ninja,
  pkg-config,
  ncnn,
  vulkan-loader,
  vapoursynth,
}:

stdenv.mkDerivation rec {
  pname = "vs-rife-ncnn-vulkan";
  version = "r9_mod_v33";

  src = fetchFromGitHub {
    owner = "styler00dollar";
    repo = "VapourSynth-RIFE-ncnn-Vulkan";
    tag = "${version}";
    hash = "sha256-9AX2D6Hl5H50ITMJ8kc/CUc8JB7qjZUMvpVmUe3b0wE=";
    fetchSubmodules = false;
  };

  dontUseCmakeConfigure = true;

  mesonBuildType = "release";

  mesonFlags = [
    (lib.mesonOption "use_system_ncnn" "true")
  ];

  nativeBuildInputs = [
    cmake
    meson
    ninja
    pkg-config
  ];

  buildInputs = [
    vulkan-loader
    vapoursynth
    ncnn
  ];

  preConfigure = ''
    substituteInPlace meson.build \
        --replace-fail "install_dir = vapoursynth_dep.get_variable(pkgconfig: 'libdir') / 'vapoursynth'" \
                        "install_dir = get_option('libdir') / 'vapoursynth'"
  '';
}
