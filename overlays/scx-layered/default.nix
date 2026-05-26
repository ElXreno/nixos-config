{ inputs, ... }:

_final: prev:
let
  rev = "bb16d0bea5c839f4de2f617bb4dc34825235a50e";
  version = "unstable-2026-05-17";
  newSrc = prev.fetchFromGitHub {
    owner = "sched-ext";
    repo = "scx";
    inherit rev;
    hash = "sha256-tOJ9SS721sqfJ1edNGC0MkHmFUJ/B82/9hylBXKz1ds=";
  };

  fenixPkgs = inputs.fenix.packages.${prev.stdenv.hostPlatform.system};
  rustPlatform = prev.makeRustPlatform {
    inherit (fenixPkgs.stable) cargo rustc;
  };

  newRustscheds = (prev.scx.rustscheds.override { inherit rustPlatform; }).overrideAttrs (oldAttrs: {
    inherit version;
    src = newSrc;
    cargoDeps = rustPlatform.fetchCargoVendor {
      pname = oldAttrs.pname or "scx_rustscheds";
      inherit version;
      src = newSrc;
      hash = "sha256-IUGLFZQrPXEQ1Owyeffc0edoYAp9AJmkG2eqQ4msy3E=";
    };

    nativeBuildInputs = (oldAttrs.nativeBuildInputs or [ ]) ++ [ prev.llvmPackages.bintools ];

    env = (oldAttrs.env or { }) // {
      RUSTFLAGS = (oldAttrs.env.RUSTFLAGS or "") + " -C link-self-contained=-linker";
    };

    preBuild = (oldAttrs.preBuild or "") + ''
      export LD_LIBRARY_PATH="${
        prev.lib.makeLibraryPath [
          prev.elfutils
          prev.zlib
          prev.zstd
        ]
      }''${LD_LIBRARY_PATH:+:$LD_LIBRARY_PATH}"
    '';

    cargoBuildFlags = [
      "--package"
      "scx_layered"
      "--package"
      "scxtop"
    ];
    cargoTestFlags = [
      "--package"
      "scx_layered"
      "--package"
      "scxtop"
    ];

    postInstall = "";

    installCheckPhase = ''
      runHook preInstallCheck
      for b in scx_layered scxtop; do
        [[ -x "$out/bin/$b" ]] || { echo "missing $out/bin/$b"; exit 1; }
      done
      runHook postInstallCheck
    '';

    passthru = (oldAttrs.passthru or { }) // {
      schedulers = [
        "scx_layered"
        "scxtop"
      ];
    };
  });
in
{
  scx = prev.scx // {
    rustscheds = newRustscheds;
    full = prev.buildEnv {
      pname = "scx_full";
      inherit (newRustscheds) version;
      paths = [
        prev.scx.cscheds
        newRustscheds
      ];
      passthru.schedulers = prev.scx.cscheds.schedulers ++ newRustscheds.passthru.schedulers;
      meta = prev.scx.full.meta;
    };
  };
}
