{
  lib,
  stdenv,
  fetchFromGitHub,
  cmake,
  ninja,
  zstd,
  lz4,
  nix-update-script,
}:

stdenv.mkDerivation (finalAttrs: {
  pname = "openzl";
  version = "0.3.0";

  src = fetchFromGitHub {
    owner = "facebook";
    repo = "openzl";
    tag = "v${finalAttrs.version}";
    hash = "sha256-qFIJhUucbGjaiKZF7UkTdaaH3VgZQtCBUABGy9A79jw=";
  };

  postUnpack = ''
    cp -rT --no-preserve=mode ${zstd.src} $sourceRoot/deps/zstd
    cp -rT --no-preserve=mode ${lz4.src} $sourceRoot/deps/lz4
  '';

  postPatch = ''
    substituteInPlace tools/CMakeLists.txt \
      --replace-fail "# Common Utilities" "set(OPENZL_BUILD_ML_SELECTOR OFF)"
  '';

  nativeBuildInputs = [
    cmake
    ninja
  ];

  cmakeFlags = [
    (lib.cmakeBool "OPENZL_BUILD_TESTS" false)
    (lib.cmakeBool "OPENZL_BUILD_BENCHMARKS" false)
    (lib.cmakeBool "OPENZL_BUILD_EXAMPLES" false)
    (lib.cmakeBool "OPENZL_BUILD_TOOLS" false)
    (lib.cmakeBool "OPENZL_BUILD_PYTHON_EXT" false)
    (lib.cmakeBool "ZSTD_BUILD_SHARED" false)
  ];

  ninjaFlags = [ "zli" ];

  installPhase = ''
    runHook preInstall
    install -Dm755 cli/zli $out/bin/zli
    runHook postInstall
  '';

  passthru.updateScript = nix-update-script { };

  meta = {
    description = "Format-aware compression framework with a universal decoder";
    homepage = "https://openzl.org";
    changelog = "https://github.com/facebook/openzl/releases/tag/v${finalAttrs.version}";
    license = lib.licenses.bsd3;
    platforms = lib.platforms.linux;
    mainProgram = "zli";
  };
})
