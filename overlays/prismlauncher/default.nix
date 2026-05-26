_:

_final: prev: {
  prismlauncher = prev.prismlauncher.override {
    jdks = with prev; [
      zulu8
      jdk21
      graalvmPackages.graalvm-oracle
    ];
  };
}
