{
  lib,
  stdenvNoCC,
  fetchFromGitHub,
  ghidra,
  nix-update-script,
}:
stdenvNoCC.mkDerivation {
  pname = "ghidra-jieli";
  version = "0-unstable-2026-03-09";

  src = fetchFromGitHub {
    owner = "quarkslab";
    repo = "ghidra-jieli";
    rev = "e1bd0707874b77b759401555d24839ad43af1267";
    hash = "sha256-7lJ9yP5qEAMASU2F3b3AV1ErZrRswEa2Hr9pk1aTznc=";
  };

  nativeBuildInputs = [ ghidra ];

  buildPhase = ''
    runHook preBuild
    export HOME=$TMPDIR
    ghidra-sleigh -a data/languages
    runHook postBuild
  '';

  installPhase = ''
    runHook preInstall

    dst=$out/lib/ghidra/Ghidra/Extensions/JieLi
    mkdir -p "$dst"
    cp -r Module.manifest data "$dst"/

    cat > "$dst/extension.properties" <<EOF
    name=JieLi
    description=Processor modules for JieLi pi32, pi32v2 and q32s cores
    author=quarkslab
    createdOn=2026-03-09
    version=${ghidra.version}
    EOF

    runHook postInstall
  '';

  passthru.updateScript = nix-update-script { extraArgs = [ "--version=branch" ]; };

  meta = {
    description = "Ghidra processor modules for JieLi custom CPU architectures";
    longDescription = ''
      Sleigh definitions for the JieLi (Zhuhai Jieli) cores used in their
      Bluetooth audio SoCs: pi32 (AC69xx), pi32v2 (BR28/BR30, e.g. AC701N)
      and q32s (BD19/BD29). Quarkslab's fork of Kagaimiq's original module,
      which extends pi32v2 to cover most instructions.
    '';
    homepage = "https://github.com/quarkslab/ghidra-jieli";
    license = lib.licenses.asl20;
    platforms = ghidra.meta.platforms;
  };
}
