{
  lib,
  rustPlatform,
  fetchFromGitHub,
  nix-update-script,
}:
rustPlatform.buildRustPackage (finalAttrs: {
  pname = "wakaru";
  version = "1.17.0";

  __structuredAttrs = true;
  strictDeps = true;

  src = fetchFromGitHub {
    owner = "pionxzh";
    repo = "wakaru";
    tag = "v${finalAttrs.version}";
    hash = "sha256-SDpB2lrbzVxqhlJyd0iF1QmK3Mj7HV+9ZFigChyfuJI=";
  };

  cargoHash = "sha256-/KrAIkGtj2vgLqEEVsEC6elHlYQrCakkMLa/9Wmfjjc=";

  cargoBuildFlags = [
    "--package"
    "wakaru-cli"
  ];
  cargoTestFlags = finalAttrs.cargoBuildFlags;

  passthru.updateScript = nix-update-script { };

  meta = {
    description = "JavaScript decompiler and bundle splitter";
    homepage = "https://github.com/pionxzh/wakaru";
    license = lib.licenses.asl20;
    platforms = lib.platforms.unix;
    mainProgram = "wakaru";
  };
})
