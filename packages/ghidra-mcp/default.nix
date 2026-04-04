{
  lib,
  stdenv,
  fetchFromGitHub,
  maven,
  ghidra,
  python3,
  makeWrapper,
  unzip,
  jdk21,
}:

let
  pname = "ghidra-mcp";
  version = "4.3.0";

  src = fetchFromGitHub {
    owner = "bethington";
    repo = "ghidra-mcp";
    rev = "v${version}";
    hash = "sha256-+37kC6Iji0Vb3NMVNdxsPbZMd6AWUX5vNqru9yooUvs=";
  };

  ghidraHome = "${ghidra}/lib/ghidra";

  ghidraVersion = ghidra.version;

  ghidraJars = {
    Base = "Ghidra/Features/Base/lib/Base.jar";
    Decompiler = "Ghidra/Features/Decompiler/lib/Decompiler.jar";
    Docking = "Ghidra/Framework/Docking/lib/Docking.jar";
    Generic = "Ghidra/Framework/Generic/lib/Generic.jar";
    Gui = "Ghidra/Framework/Gui/lib/Gui.jar";
    FileSystem = "Ghidra/Framework/FileSystem/lib/FileSystem.jar";
    Help = "Ghidra/Framework/Help/lib/Help.jar";
    Project = "Ghidra/Framework/Project/lib/Project.jar";
    SoftwareModeling = "Ghidra/Framework/SoftwareModeling/lib/SoftwareModeling.jar";
    Utility = "Ghidra/Framework/Utility/lib/Utility.jar";
  };

  installGhidraJars = repoDir: ''
    ${lib.concatStringsSep "\n" (
      lib.mapAttrsToList (name: jarPath: ''
        mvn -q install:install-file \
          -Dmaven.repo.local="${repoDir}" \
          -Dfile="${ghidraHome}/${jarPath}" \
          -DgroupId=ghidra \
          -DartifactId=${name} \
          -Dversion=${ghidraVersion} \
          -Dpackaging=jar \
          -DgeneratePom=true
      '') ghidraJars
    )}
  '';

  patchPomVersion = ''
    sed -i "s|<ghidra.version>[^<]*</ghidra.version>|<ghidra.version>${ghidraVersion}</ghidra.version>|" pom.xml
  '';

  mavenDeps = stdenv.mkDerivation {
    pname = "${pname}-maven-deps";
    inherit src version;
    nativeBuildInputs = [
      maven
      jdk21
    ];

    buildPhase = ''
      runHook preBuild
      mkdir -p "$out/.m2"
      ${installGhidraJars "$out/.m2"}
      ${patchPomVersion}
      mvn -Dmaven.repo.local="$out/.m2" -DskipTests clean package assembly:single
      runHook postBuild
    '';

    installPhase = ''
      runHook preInstall
      find "$out" -type f \( \
        -name \*.lastUpdated \
        -o -name resolver-status.properties \
        -o -name _remote.repositories \) \
        -delete
      runHook postInstall
    '';

    dontFixup = true;
    outputHashAlgo = "sha256";
    outputHashMode = "recursive";
    outputHash = "sha256-xoNH22gn/YZiBo8vxJIwQjuhHhrCyGprfyS3x7cLWMo=";
  };

  pythonEnv = python3.withPackages (
    ps: with ps; [
      mcp
      requests
    ]
  );

in
stdenv.mkDerivation {
  inherit pname version src;

  nativeBuildInputs = [
    maven
    jdk21
    makeWrapper
    unzip
  ];

  buildPhase = ''
    runHook preBuild
    ${patchPomVersion}
    cp -r "${mavenDeps}/.m2" "$TMPDIR/m2"
    chmod -R u+w "$TMPDIR/m2"
    mvn -o -Dmaven.repo.local=$TMPDIR/m2 -DskipTests clean package assembly:single
    runHook postBuild
  '';

  installPhase = ''
    runHook preInstall
    mkdir -p $out/lib/ghidra/Ghidra/Extensions
    zipFile=$(echo target/*.zip)
    unzip -d $out/lib/ghidra/Ghidra/Extensions "$zipFile"
    install -Dm644 bridge_mcp_ghidra.py $out/share/ghidra-mcp/bridge_mcp_ghidra.py
    makeWrapper ${pythonEnv}/bin/python $out/bin/bridge_mcp_ghidra \
      --add-flags "$out/share/ghidra-mcp/bridge_mcp_ghidra.py"
    runHook postInstall
  '';

  meta = with lib; {
    description = "MCP server + Ghidra plugin for AI-powered reverse engineering";
    mainProgram = "bridge_mcp_ghidra";
    homepage = "https://github.com/bethington/ghidra-mcp";
    license = licenses.asl20;
    platforms = platforms.unix;
  };
}
