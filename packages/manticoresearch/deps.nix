{
  lib,
  stdenv,
  cmake,
  pkg-config,
  fetchFromGitHub,
  simde,
  mclSrc,
  extraCFlags ? "",
}:

let
  mkCmakeLib =
    {
      pname,
      version,
      src,
      license,
      env ? { },
      ...
    }@args:
    stdenv.mkDerivation (
      {
        nativeBuildInputs = [
          cmake
          pkg-config
        ];
        meta = {
          inherit license;
          platforms = lib.platforms.all;
        };
      }
      // removeAttrs args [ "license" ]
      // lib.optionalAttrs (extraCFlags != "") {
        env = env // {
          NIX_CFLAGS_COMPILE = lib.concatStringsSep " " (
            lib.filter (f: f != "") [
              (env.NIX_CFLAGS_COMPILE or "")
              extraCFlags
            ]
          );
        };
      }
    );
in
rec {
  hnswlib = mkCmakeLib {
    pname = "manticore-hnswlib";
    version = "0-unstable-1e1bffe";
    src = fetchFromGitHub {
      owner = "manticoresoftware";
      repo = "hnswlib";
      rev = "1e1bffe6191f55d92b046170a79d39e4b4463318";
      hash = "sha256-+l7YdVa7t8ilzYfekkD1PziBdImoqcbAigYZtkr84M4=";
    };
    license = lib.licenses.asl20;
  };

  pgm-index = mkCmakeLib {
    pname = "manticore-pgm-index";
    version = "pgm_2022_08_02";
    src = fetchFromGitHub {
      owner = "manticoresoftware";
      repo = "PGM-index";
      rev = "5a5a763c7c07e56f56fd33137dcee9f1b3c3b640";
      hash = "sha256-p8CjY9r1TEi91HBac9rZ39Yae2XuNh2yQWbo65n90jU=";
    };
    postPatch = "cp ${mclSrc}/pgm/CMakeLists.txt CMakeLists.txt";
    license = lib.licenses.asl20;
  };

  streamvbyte = mkCmakeLib {
    pname = "manticore-streamvbyte";
    version = "0-unstable-efdd9da";
    src = fetchFromGitHub {
      owner = "manticoresoftware";
      repo = "streamvbyte";
      rev = "efdd9dace81a4a8f844267631879b500c6d913cf";
      hash = "sha256-a9E1aWBY/P7wI+kgHqhEiD3THctFfeFcy658RcNpHfQ=";
    };
    env.NIX_CFLAGS_COMPILE = lib.optionalString stdenv.hostPlatform.isx86_64 "-msse4.1";
    postPatch = "cp ${mclSrc}/streamvbyte/CMakeLists.txt CMakeLists.txt";
    license = lib.licenses.asl20;
  };

  fastpfor = mkCmakeLib {
    pname = "manticore-fastpfor";
    version = "0-unstable-5f6b7df";
    src = fetchFromGitHub {
      owner = "manticoresoftware";
      repo = "FastPFor";
      rev = "5f6b7df8f0dba402bb616a81ca510bfeb04c42ce";
      hash = "sha256-O9nqgsZKT+q9dHdh2RahF1U8KXCmDp0ld7hNvVREM8k=";
    };
    buildInputs = [ simde ];
    env.NIX_CFLAGS_COMPILE = lib.optionalString stdenv.hostPlatform.isx86_64 "-msse3 -mssse3 -msse4.1";
    postPatch = ''
      cp ${mclSrc}/libfastpfor/CMakeLists.txt CMakeLists.txt
      sed -i '/Add simde for arm/,/FetchContent_GetProperties ( simde )/c\
        set(simde_SOURCE_DIR "${simde.src}")\n\
        set(simde_POPULATED 1)\n\
      ' CMakeLists.txt
    '';
    license = lib.licenses.asl20;
  };

  uni-algo = mkCmakeLib {
    pname = "manticore-uni-algo";
    version = "0.7.2";
    src = fetchFromGitHub {
      owner = "manticoresoftware";
      repo = "uni-algo";
      rev = "v0.7.2";
      hash = "sha256-+V9w4UJ+3KsyZUYht6OEzms60mBHd8FewVc7f21Z9ww=";
    };
    license = lib.licenses.mit;
  };

  cctz = mkCmakeLib {
    pname = "manticore-cctz";
    version = "0-unstable-8ca8e32";
    src = fetchFromGitHub {
      owner = "manticoresoftware";
      repo = "cctz";
      rev = "8ca8e325cb3d8a0b768f7530d0e31a907982c6d4";
      hash = "sha256-+haCZNKZBKRMCn3fyX29wRrtI/YecFGZ+6l/i8xNtOk=";
    };
    cmakeBuildDir = "build_dir";
    license = lib.licenses.asl20;
  };

  xxhash = mkCmakeLib {
    pname = "manticore-xxhash";
    version = "0-unstable-72997b0";
    src = fetchFromGitHub {
      owner = "manticoresoftware";
      repo = "xxHash";
      rev = "72997b0070a031c063f86a308aec77ae742706d3";
      hash = "sha256-QAIxZeMiohm/BYyO0f70En6GOv7t3yLH2pJfkUek7Js=";
    };
    license = lib.licenses.bsd2;
  };

  limonp = fetchFromGitHub {
    owner = "yanyiwu";
    repo = "limonp";
    tag = "v0.9.0";
    hash = "sha256-/8ZjF5M4jEGYRLziTlRNCGKgtIQS5XrFqB9Hv7wbBKw=";
  };

  jieba = mkCmakeLib {
    pname = "manticore-cppjieba";
    version = "0-unstable-0fa3cb0";
    src = fetchFromGitHub {
      owner = "manticoresoftware";
      repo = "cppjieba";
      rev = "0fa3cb07232f5c144228c2e8e39521bfdc9d3bca";
      hash = "sha256-THuUxNS9t7JEEWfRJrvNa9O3s7eOCqIA3ztg1wyvnDA=";
    };
    postPatch = ''
      substituteInPlace CMakeLists.txt \
        --replace-fail '"''${FETCHCONTENT_BASE_DIR}/limunp-src/include/"' '"${limonp}/include/"'
    '';
    cmakeFlags = [
      "-DNO_BUILD=1"
      "-DFETCHCONTENT_FULLY_DISCONNECTED=ON"
      "-DFETCHCONTENT_SOURCE_DIR_LIMUNP=${limonp}"
    ];
    license = lib.licenses.mit;
  };
}
