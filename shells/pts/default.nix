{
  pkgs,
  ...
}:
let
  fhs = pkgs.buildFHSEnv {
    name = "pts-env";
    targetPkgs =
      _pkgs: with pkgs; [
        phoronix-test-suite
        php

        stdenv.cc
        gcc
        gcc.cc
        gnumake
        cmake
        ninja
        autoconf
        automake
        libtool
        pkg-config
        bison
        flex
        bc
        binutils
        coreutils
        findutils
        gnused
        gawk
        gnugrep
        diffutils
        patch
        which
        file
        gnutar
        gzip
        bzip2
        xz
        zstd
        lz4
        unzip
        p7zip

        perl
        python3

        git
        wget
        curl
        cacert

        openssl
        openssl.dev
        zlib
        zlib.dev
        libelf
        elfutils
        ncurses
        ncurses.dev
        libxml2
        glibc.static
        nasm
        yasm

        mesa-demos
      ];
    multiPkgs =
      _pkgs: with pkgs; [
        zlib
        ncurses
        libgcc
        glibc.dev
      ];
    runScript = "fish";
    profile = ''
      export NIX_ENFORCE_NO_NATIVE=0
      mkdir -p "$HOME/.phoronix-test-suite"
      if [ ! -f "$HOME/.phoronix-test-suite/user-config.xml" ]; then
        cp ${./user-config.xml} "$HOME/.phoronix-test-suite/user-config.xml"
        chmod u+w "$HOME/.phoronix-test-suite/user-config.xml"
      fi
    '';
  };
in
pkgs.stdenv.mkDerivation {
  name = "pts-env-shell";
  nativeBuildInputs = [ fhs ];
  shellHook = "exec pts-env";
}
