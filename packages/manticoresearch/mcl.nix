{
  lib,
  stdenv,
  callPackage,
  cmake,
  mclSrc,
  mclVersion,
  variant,
}:

let
  marchFlags = {
    avx2 = "-march=x86-64-v3";
    avx512 = "-march=x86-64-v4 -mavx512vpopcntdq";
  };

  deps = callPackage ./deps.nix {
    inherit mclSrc;
    extraCFlags = marchFlags.${variant};
  };
in
stdenv.mkDerivation {
  pname = "manticore-columnar-${variant}";
  version = mclVersion;

  src = mclSrc;

  nativeBuildInputs = [ cmake ];

  buildInputs = with deps; [
    fastpfor
    hnswlib
    pgm-index
    streamvbyte
  ];

  postPatch = ''
    substituteInPlace CMakeLists.txt \
      --replace-fail 'project ( columnar VERSION 0.0.0 )' 'project ( columnar VERSION ${mclVersion} )'
  '';

  cmakeFlags = [
    (lib.cmakeBool "EXTERNAL_LIB" true)
    (lib.cmakeBool "BUILD_TESTING" false)
  ]
  ++ lib.optional (variant == "avx512") (lib.cmakeBool "USE_AVX512" true);

  meta = {
    description = "Manticore Columnar Library modules built for ${variant}";
    homepage = "https://github.com/manticoresoftware/columnar";
    license = lib.licenses.asl20;
    platforms = [ "x86_64-linux" ];
  };
}
