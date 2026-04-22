{
  lib,
  stdenvNoCC,
  fetchurl,
}:
stdenvNoCC.mkDerivation (finalAttrs: {
  pname = "lovelace-xiaomi-vacuum-map-card";
  version = "2.3.2";

  src = fetchurl {
    url = "https://github.com/PiotrMachowski/lovelace-xiaomi-vacuum-map-card/releases/download/v${finalAttrs.version}/xiaomi-vacuum-map-card.js";
    hash = "sha256-q7VCN9EgbiEV/pmuTelRWLS+QMUzuGeSXvnttFfWiuA=";
  };

  dontUnpack = true;

  installPhase = ''
    runHook preInstall
    install -Dm644 $src $out/xiaomi-vacuum-map-card.js
    runHook postInstall
  '';

  passthru.entrypoint = "xiaomi-vacuum-map-card.js";

  meta = {
    changelog = "https://github.com/PiotrMachowski/lovelace-xiaomi-vacuum-map-card/releases/tag/v${finalAttrs.version}";
    description = "Xiaomi Vacuum Map card for Home Assistant Lovelace UI";
    homepage = "https://github.com/PiotrMachowski/lovelace-xiaomi-vacuum-map-card";
    license = lib.licenses.mit;
    maintainers = with lib.maintainers; [ elxreno ];
    platforms = lib.platforms.all;
    sourceProvenance = with lib.sourceTypes; [ binaryBytecode ];
  };
})
