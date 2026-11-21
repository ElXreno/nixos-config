{
  lib,
  stdenv,
  callPackage,
  fetchFromGitHub,
  fetchzip,
  bison,
  cmake,
  flex,
  pkg-config,
  boost187,
  croaring,
  curl,
  expat,
  icu,
  libstemmer,
  mariadb-connector-c,
  nlohmann_json,
  openssl,
  re2,
  zlib,
  zstd,
  tzdata,
  versionCheckHook,
}:

let
  version = "29.9.0";
  mclVersion = "14.2.1";

  src = fetchFromGitHub {
    owner = "manticoresoftware";
    repo = "manticoresearch";
    tag = version;
    fetchSubmodules = true;
    hash = "sha256-MyxrJJ/FVCl/U5aRCL0JFxgHMrGCJohAM3u96md3WOY=";
  };

  mclSrc = "${src}/mcl";

  deps = callPackage ./deps.nix { inherit mclSrc; };

  mclVariants = lib.optionalAttrs stdenv.hostPlatform.isx86_64 (
    lib.genAttrs [ "avx2" "avx512" ] (
      variant: callPackage ./mcl.nix { inherit mclSrc mclVersion variant; }
    )
  );

  embeddings = callPackage ./embeddings.nix { inherit mclSrc; };

  lemmatizers =
    lib.mapAttrs
      (
        lang: hash:
        fetchzip {
          name = "manticore-lemmatizer-${lang}";
          url = "https://repo.manticoresearch.com/repository/morphology/${lang}.pak.tgz";
          stripRoot = false;
          inherit hash;
        }
      )
      {
        de = "sha256-iwxwWnuB7i6BFlBaGe363QOIjJVZAFPI/rLI4hbvyds=";
        en = "sha256-6DUOhVck1e6zTkj2mUvr3PMfmYnxi2sRaASOiJXccKI=";
        ru = "sha256-kvgzE/Vmd3PklOFlT90oStltp3tl01tC0JVevD++mXM=";
      };
in
stdenv.mkDerivation (finalAttrs: {
  pname = "manticoresearch";
  inherit version src;

  nativeBuildInputs = [
    bison
    cmake
    flex
    pkg-config
  ];

  buildInputs = [
    boost187
    croaring
    curl
    expat
    icu.dev
    libstemmer
    mariadb-connector-c
    nlohmann_json
    openssl
    re2
    zlib
    zstd
  ]
  ++ (with deps; [
    cctz
    fastpfor
    hnswlib
    jieba
    pgm-index
    streamvbyte
    uni-algo
    xxhash
  ]);

  patches = [
    ./gcc-using-access.patch
    ./lib-postfix-last-dot.patch
    ./tzdir-realpath.patch
  ];

  stripAllList = [
    "bin"
    "share/manticore/modules"
  ];

  postPatch = ''
    substituteInPlace src/CMakeLists.txt \
      --replace-fail 'set ( Boost_USE_STATIC_LIBS ON )' 'set ( Boost_USE_STATIC_LIBS OFF )'
    substituteInPlace CMakeLists.txt \
      --replace-fail 'set ( OPENSSL_USE_STATIC_LIBS ON )' 'set ( OPENSSL_USE_STATIC_LIBS OFF )'
    substituteInPlace src/sphinxversion.h.in \
      --replace-fail '0.0.0' '${finalAttrs.version}'
    substituteInPlace mcl/CMakeLists.txt \
      --replace-fail 'project ( columnar VERSION 0.0.0 )' 'project ( columnar VERSION ${mclVersion} )'
    substituteInPlace mcl/knn/CMakeLists.txt \
      --replace-fail 'if ( NOT EXTERNAL_LIB ) # install only once' 'if ( NOT EXTERNAL_LIB AND EMBEDDINGS_LIB ) # install only once'
  '';

  cmakeFlags = [
    (lib.cmakeBool "WITH_GALERA" false)
    (lib.cmakeBool "WITH_ICU" true)
    (lib.cmakeBool "WITH_ICU_FORCE_STATIC" false)
    (lib.cmakeBool "WITH_RE2_FORCE_STATIC" false)
    (lib.cmakeBool "WITH_STEMMER_FORCE_STATIC" false)
    (lib.cmakeBool "WITH_MYSQL" true)
    (lib.cmakeBool "WITH_ZSTD" true)
    (lib.cmakeBool "DL_ZSTD" false)
    (lib.cmakeBool "DL_EXPAT" false)
    (lib.cmakeBool "DL_CURL" false)
    (lib.cmakeBool "MCL_LOCAL_RUNTIME" true)
    (lib.cmakeBool "BUILD_EMBEDDINGS_LOCALLY" false)
    (lib.cmakeBool "BUILD_TESTING" false)
    (lib.cmakeFeature "MYSQL_INCLUDE_DIR" "${mariadb-connector-c.dev}/include/mariadb")
    (lib.cmakeFeature "MYSQL_LIB" "${mariadb-connector-c.out}/lib/mariadb/libmariadb.so")
    (lib.cmakeFeature "CONFDIR" "${placeholder "out"}/etc")
    (lib.cmakeFeature "LOGDIR" "/var/lib/manticore/log")
    (lib.cmakeFeature "RUNDIR" "/run/manticore")
    (lib.cmakeFeature "DISTR" "nixpkgs")
  ];

  postInstall = ''
    share=$out/share/manticore
    ${lib.concatMapStrings (l: ''
      install -Dm644 ${l}/*.pak -t $share
    '') (lib.attrValues lemmatizers)}
    mkdir -p $share/jieba
    cp ${deps.jieba}/dict/*.utf8 $share/jieba/
    ln -s ${tzdata}/share/zoneinfo $share/tzdata
    ${lib.concatMapStrings (v: ''
      install -Dm755 ${v}/share/manticore/modules/*.so -t $share/modules
    '') (lib.attrValues mclVariants)}

    install -Dm644 ${finalAttrs.src}/dist/deb/manticore.service.in $out/lib/systemd/system/manticore.service
    substituteInPlace $out/lib/systemd/system/manticore.service \
      --replace-fail "@CMAKE_INSTALL_FULL_BINDIR@" "$out/bin" \
      --replace-fail "@CMAKE_INSTALL_FULL_SYSCONFDIR@" "$out/etc"
  '';

  doInstallCheck = true;
  nativeInstallCheckInputs = [ versionCheckHook ];
  versionCheckProgram = "${placeholder "out"}/bin/searchd";

  passthru = {
    inherit
      deps
      embeddings
      lemmatizers
      mclVariants
      ;
  };

  meta = {
    description = "Open source search database with columnar storage, secondary indexes and KNN";
    homepage = "https://manticoresearch.com";
    changelog = "https://github.com/manticoresoftware/manticoresearch/releases/tag/${finalAttrs.version}";
    license = with lib.licenses; [
      gpl3Plus
      asl20
    ];
    mainProgram = "searchd";
    platforms = lib.platforms.linux;
  };
})
