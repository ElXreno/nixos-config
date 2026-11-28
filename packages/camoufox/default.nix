{
  lib,
  stdenv,
  fetchurl,
  unzip,
  autoPatchelfHook,
  patchelfUnstable,
  gtk3,
  alsa-lib,
  dbus-glib,
  libxtst,
  libva,
  pciutils,
  curl,
  pipewire,
  gsettings-desktop-schemas,
  buildNpmPackage,
  nodejs_24,
  makeWrapper,
}:
let
  camoufox = stdenv.mkDerivation (finalAttrs: {
    pname = "camoufox";
    version = "158.0.1-beta.41";

    src = fetchurl {
      url = "https://github.com/daijro/camoufox/releases/download/v${finalAttrs.version}/camoufox-${finalAttrs.version}-lin.x86_64.zip";
      hash = "sha256-Py6x0EUfpY5F5D5V9eYYFWUGl5GuYMBXTwzrHwE0HDM=";
    };

    unpackCmd = "unzip -q $curSrc -d source";
    sourceRoot = "source";

    nativeBuildInputs = [
      unzip
      autoPatchelfHook
      patchelfUnstable
    ];

    buildInputs = [
      gtk3
      alsa-lib
      dbus-glib
      libxtst
    ];

    runtimeDependencies = [
      curl
      pciutils
      libva.out
    ];

    appendRunpaths = [ "${pipewire}/lib" ];

    patchelfFlags = [ "--no-clobber-old-sections" ];

    installPhase = ''
      runHook preInstall
      mkdir -p $out/lib/camoufox $out/bin
      cp -r . $out/lib/camoufox
      ln -s $out/lib/camoufox/camoufox-bin $out/bin/camoufox
      runHook postInstall
    '';

    meta = {
      description = "Anti-detect Firefox build for web scraping and AI agents";
      homepage = "https://github.com/daijro/camoufox";
      license = lib.licenses.mpl20;
      platforms = [ "x86_64-linux" ];
      sourceProvenance = [ lib.sourceTypes.binaryNativeCode ];
      mainProgram = "camoufox";
    };
  });

  camoufox-cli = buildNpmPackage {
    pname = "camoufox-cli";
    version = "0.1.0";

    src = ./cli;

    nodejs = nodejs_24;
    npmDepsHash = "sha256-pk9IOyl9lD1extTJZDJjaDkUh/0A4B/X6+mSlgA5SLE=";
    npmFlags = [ "--ignore-scripts" ];
    dontNpmBuild = true;

    nativeBuildInputs = [ makeWrapper ];

    postInstall = ''
      wrapProgram $out/bin/camoufox-cli \
        --set CAMOUFOX_EXECUTABLE_PATH ${camoufox}/lib/camoufox/camoufox-bin \
        --set PLAYWRIGHT_SKIP_BROWSER_DOWNLOAD 1 \
        --prefix XDG_DATA_DIRS : ${gsettings-desktop-schemas}/share/gsettings-schemas/${gsettings-desktop-schemas.name}:${gtk3}/share/gsettings-schemas/${gtk3.name}
      mkdir -p $out/share/camoufox-cli/skills
      cp -r $out/lib/node_modules/camoufox-cli/node_modules/@playwright/cli/skills/playwright-cli $out/share/camoufox-cli/skills/camoufox
      chmod -R u+w $out/share/camoufox-cli/skills/camoufox
      find $out/share/camoufox-cli/skills/camoufox -type f -name '*.md' -exec sed -i 's/playwright-cli/camoufox-cli/g' {} +
      awk 'NR == 1 && /^---$/ { fm = 1; next } fm && /^---$/ { fm = 0; next } !fm' \
        $out/share/camoufox-cli/skills/camoufox/SKILL.md > upstream.md
      cat ${./skill-header.md} upstream.md > $out/share/camoufox-cli/skills/camoufox/SKILL.md
    '';

    meta = {
      description = "playwright-cli driving Camoufox with a fresh fingerprint per session";
      license = lib.licenses.asl20;
      platforms = [ "x86_64-linux" ];
      mainProgram = "camoufox-cli";
    };
  };
in
{
  inherit camoufox camoufox-cli;
}
