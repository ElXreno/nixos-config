_:

_final: prev: {
  prismlauncher = prev.prismlauncher.override {
    jdks = with prev; [
      graalvmPackages.graalvm-oracle
    ];
  };
}
