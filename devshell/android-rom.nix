# Source: https://gist.github.com/Arian04/bea169c987d46a7f51c63a68bc117472/de9d655e395c15f1bcd7806aff10012d03880646

# I used this shell.nix to build LineageOS 21.0 for redfin (Google Pixel 5)
#
# IMPORANT NOTE: I had to use a pure shell for my builds to work, i.e: `nix-shell --pure` rather than `nix-shell`
#
# The build instructions are here: https://wiki.lineageos.org/devices/redfin/build
#
# Warning (from forked gist, was added August 1st 2018):
# The hardened NixOS kernel disables 32 bit emulation, which made me run into multiple "Exec format error" errors.
# To fix, use the default kernel, or enable "IA32_EMULATION y" in the kernel config.
#
# Created using:
#   https://gist.github.com/Nadrieril/d006c0d9784ba7eff0b092796d78eb2a
#   https://nixos.wiki/wiki/Android#Building_Android_on_NixOS

{
  pkgs,
  ...
}:
let
  fhs = pkgs.buildFHSEnv {
    name = "android-env";
    targetPkgs =
      pkgs: with pkgs; [
        android-tools
        libxcrypt-legacy # libcrypt.so.1
        freetype # libfreetype.so.6
        fontconfig # java NPE: "sun.awt.FontConfiguration.head" is null
        yaml-cpp # necessary for some kernels according to a comment on the gist

        # Some of the packages here are probably unecessary but I don't wanna figure out which
        bc
        binutils
        bison
        ccache
        curl
        flex
        gcc
        git
        git-repo
        git-lfs
        gnumake
        gnupg
        gperf
        imagemagick
        jdk11
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

        libgcc # crtbeginS.o
        iconv # ??? - i put this here and by the time i went back to remove unecessary packages i forgot why
        iconv.dev # sys/types.h
      ];
    runScript = "bash";
    profile = ''
      export ALLOW_NINJA_ENV=true
      export USE_CCACHE=1
      export CCACHE_EXEC=/usr/bin/ccache
      export ANDROID_JAVA_HOME=${pkgs.jdk11.home}
      # Building involves a phase of unzipping large files into a temporary directory
      export TMPDIR=/tmp
      export LD_LIBRARY_PATH=$LD_LIBRARY_PATH:${pkgs.ncurses5}/lib

      cd /mnt/android
    '';
  };
in
pkgs.stdenv.mkDerivation {
  name = "android-env-shell";
  nativeBuildInputs = [ fhs ];
  shellHook = "exec android-env";
}
