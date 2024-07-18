{ pkgs, ... }:

(pkgs.buildFHSUserEnv {
  name = "fhs-android";
  targetPkgs = pkgs:
    (with pkgs; [
      bison
      curl
      flex
      git
      gitRepo
      gnumake
      gnupg
      gperf
      jdk
      libxml2
      lzop
      m4
      nettools
      openssl
      perl
      procps
      python2
      python3
      schedtool
      unzip
      utillinux
      zip
      pkgconfig
    ]);
  multiPkgs = pkgs: (with pkgs; [ ncurses5 zlib glibc.dev ]);
  runScript = "bash";
  profile = ''
    export ALLOW_NINJA_ENV=true
  '';
}).env
