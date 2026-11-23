{
  lib,
  fetchurl,
  graalvmPackages,
  makeWrapper,
  stdenvNoCC,
  unzip,
}:
stdenvNoCC.mkDerivation (finalAttrs: {
  pname = "gtnh-server";
  version = "2.9.0-RC-2";

  src = fetchurl {
    url = "https://downloads.gtnewhorizons.com/ServerPacks/betas/GT_New_Horizons_${finalAttrs.version}_Server_Java_17-26.zip";
    hash = "sha256-4AWg0FDsBgc7Ow0RsUm1q5NVys9LjEuDfFC4qJlG2/k=";
  };

  nativeBuildInputs = [
    makeWrapper
    unzip
  ];

  unpackPhase = ''
    runHook preUnpack
    unzip -q "$src" -d source
    runHook postUnpack
  '';

  installPhase = ''
    runHook preInstall
    cd source
    mkdir -p $out
    cp -r \
      config \
      libraries \
      mods \
      serverutilities \
      forge-1.7.10-10.13.4.1614-1.7.10-universal.jar \
      java9args.txt \
      lwjgl3ify-forgePatches.jar \
      minecraft_server.1.7.10.jar \
      server-icon.png \
      $out/
    makeWrapper ${graalvmPackages.graalvm-oracle}/bin/java $out/bin/${finalAttrs.meta.mainProgram} \
      --append-flags "@$out/java9args.txt -jar $out/lwjgl3ify-forgePatches.jar nogui"
    runHook postInstall
  '';

  meta = {
    description = "GT New Horizons Minecraft server";
    homepage = "https://www.gtnewhorizons.com";
    license = lib.licenses.unfreeRedistributable;
    platforms = lib.platforms.unix;
    mainProgram = "minecraft-server";
  };
})
