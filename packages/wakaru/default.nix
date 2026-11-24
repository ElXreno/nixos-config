{
  lib,
  rustPlatform,
  fetchFromGitHub,
  nix-update-script,
}:
rustPlatform.buildRustPackage (finalAttrs: {
  pname = "wakaru";
  version = "1.14.0";

  __structuredAttrs = true;
  strictDeps = true;

  src = fetchFromGitHub {
    owner = "pionxzh";
    repo = "wakaru";
    tag = "v${finalAttrs.version}";
    hash = "sha256-HdSrirk4VcWlifK4m97fJbyfXeIhLfzWRWQGPxAd0hI=";
  };

  cargoHash = "sha256-t3LS3nMihmHmRmJ014YkvXYynv4XwtQWn267GAWnLiE=";

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
