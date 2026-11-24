{
  lib,
  stdenvNoCC,
  fetchurl,
}:
stdenvNoCC.mkDerivation (finalAttrs: {
  pname = "jadx-headless-mcp";
  version = "0.7.1";

  src = fetchurl {
    url = "https://github.com/1013503897/jadx-headless-mcp/releases/download/v${finalAttrs.version}/jadx-headless-mcp-${finalAttrs.version}-all.jar";
    hash = "sha256-bl6s9QC2QpK/tzxJeXwZWPbuRGRuQ+hoA5rn/rVz/3U=";
  };

  dontUnpack = true;

  installPhase = ''
    runHook preInstall
    install -Dm444 $src $out/share/java/jadx-headless-mcp.jar
    runHook postInstall
  '';

  passthru.jar = "${finalAttrs.finalPackage}/share/java/jadx-headless-mcp.jar";

  meta = {
    description = "Headless MCP server for Android APK static analysis built on jadx-core";
    homepage = "https://github.com/1013503897/jadx-headless-mcp";
    license = lib.licenses.unfree;
    sourceProvenance = [ lib.sourceTypes.binaryBytecode ];
    platforms = lib.platforms.all;
  };
})
