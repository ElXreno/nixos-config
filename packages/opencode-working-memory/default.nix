{
  lib,
  stdenvNoCC,
  fetchFromGitHub,
  typescript,
}:
stdenvNoCC.mkDerivation (finalAttrs: {
  pname = "opencode-working-memory";
  version = "1.6.9";

  src = fetchFromGitHub {
    owner = "sdwolf4103";
    repo = "opencode-working-memory";
    tag = "v${finalAttrs.version}";
    hash = "sha256-blBzxD5EXU1IGN6+kvvfdaLISD6J6ZtQXzJkT+Xm9vA=";
  };

  nativeBuildInputs = [ typescript ];

  buildPhase = ''
    runHook preBuild
    tsc -p tsconfig.memory-diag.json || true
    test -f dist/index.js
    runHook postBuild
  '';

  installPhase = ''
    runHook preInstall
    mkdir -p $out/lib/node_modules/${finalAttrs.pname}
    cp -r dist package.json README.md LICENSE $out/lib/node_modules/${finalAttrs.pname}/
    runHook postInstall
  '';

  passthru.pluginDir = "${finalAttrs.finalPackage}/lib/node_modules/${finalAttrs.pname}";

  meta = {
    description = "Three-layer memory architecture for OpenCode with workspace memory and hot session state";
    homepage = "https://github.com/sdwolf4103/opencode-working-memory";
    license = lib.licenses.mit;
    maintainers = with lib.maintainers; [ elxreno ];
    platforms = lib.platforms.all;
  };
})
