_:

_final: prev: {
  prismlauncher = prev.prismlauncher.override {
    jdks = with prev; [
      (zulu8.override { gtkSupport = false; })
      jdk21
      graalvmPackages.graalvm-oracle
    ];
    openal = prev.openal.overrideAttrs (
      finalAttrs: _: {
        version = "1.25.2";
        src = prev.fetchFromGitHub {
          owner = "kcat";
          repo = "openal-soft";
          tag = finalAttrs.version;
          hash = "sha256-+yG5qB5sg86RgmFIaPNNsLZZ1IM0AvNdzWPVN/PQLGw=";
        };
      }
    );
  };
}
