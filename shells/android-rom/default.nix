# based on https://gist.github.com/Arian04/bea169c987d46a7f51c63a68bc117472
{
  pkgs,
  ...
}:
let
  inherit (pkgs) lib;

  buildRoot = "/mnt/scratch/android-builds";
  ccacheDir = "${buildRoot}/ccache";

  withO3 =
    drv:
    drv.overrideAttrs (old: {
      env = (old.env or { }) // {
        NIX_CFLAGS_COMPILE = toString [
          (old.env.NIX_CFLAGS_COMPILE or "")
          "-O3"
        ];
      };
    });

  ccache =
    (withO3 (
      pkgs.ccache.override {
        zstd = withO3 pkgs.zstd;
        xxhash = withO3 pkgs.xxhash;
      }
    )).overrideAttrs
      (old: {
        cmakeFlags = old.cmakeFlags ++ [ (lib.cmakeBool "CMAKE_INTERPROCEDURAL_OPTIMIZATION" true) ];
      });

  ccacheConf = pkgs.writeText "ccache.conf" ''
    max_size = 150G
  '';

  fhs = pkgs.buildFHSEnv {
    name = "android-env";
    targetPkgs =
      _pkgs: with pkgs; [
        ccache
        git
        git-repo
        git-lfs

        android-tools
        brotli
        e2fsprogs
        libxcrypt-legacy
        freetype
        fontconfig
        yaml-cpp

        bc
        binutils
        bison
        curl
        flex
        gcc
        gcc.cc
        gnumake
        gnupg
        gperf
        imagemagick
        jdk
        elfutils
        libxml2
        libxslt
        lz4
        lzop
        m4
        nettools
        openssl.dev
        perl
        pngcrush
        procps
        python3
        rsync
        schedtool
        SDL
        squashfsTools
        unzip
        util-linux
        xml2
        zip
        openssl
        unixtools.xxd
        toybox

        fish
      ];
    multiPkgs =
      pkgs: with pkgs; [
        zlib
        ncurses5
        libcxx
        readline

        libgcc
        iconv
        iconv.dev
        glibc.dev
      ];
    runScript = "bash";
    profile = ''
      unset DIRENV_DIR DIRENV_FILE DIRENV_DIFF DIRENV_WATCHES

      export LANG=C.UTF-8
      export LC_ALL=C.UTF-8

      export USE_CCACHE=1
      export CCACHE_EXEC=/usr/bin/ccache
      export CCACHE_DIR=${ccacheDir}
      mkdir -p "$CCACHE_DIR"
      ln -sfn ${ccacheConf} "$CCACHE_DIR/ccache.conf"

      export ROOMSERVICE_DRYRUN=true
      export SKIP_ABI_CHECKS=true
      export USE_THINLTO_CACHE=true

      case "$PWD" in
        ${buildRoot}*) ;;
        *) cd ${buildRoot} ;;
      esac
    '';
  };
in
pkgs.mkShell {
  name = "android-env-shell";
  nativeBuildInputs = [ fhs ];
  shellHook = "exec android-env";
}
