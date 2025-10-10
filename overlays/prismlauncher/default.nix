_:

_final: prev: {
  prismlauncher = prev.prismlauncher.override {
    jdks = with prev; [
      jdk17
      graalvmPackages.graalvm-oracle
      jdk24
      jre8
      zulu
    ];
  };
}
