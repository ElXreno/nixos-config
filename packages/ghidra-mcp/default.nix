{
  lib,
  maven,
  ghidra,
  python3,
  makeWrapper,
  unzip,
  jdk21,
  fetchFromGitHub,
}:

let
  ghidraHome = "${ghidra}/lib/ghidra";
  ghidraVersion = ghidra.version;

  ghidraJars = {
    Base = "Ghidra/Features/Base/lib/Base.jar";
    DB = "Ghidra/Framework/DB/lib/DB.jar";
    Debugger-api = "Ghidra/Debug/Debugger-api/lib/Debugger-api.jar";
    Debugger-rmi-trace = "Ghidra/Debug/Debugger-rmi-trace/lib/Debugger-rmi-trace.jar";
    Decompiler = "Ghidra/Features/Decompiler/lib/Decompiler.jar";
    Docking = "Ghidra/Framework/Docking/lib/Docking.jar";
    Emulation = "Ghidra/Framework/Emulation/lib/Emulation.jar";
    FileSystem = "Ghidra/Framework/FileSystem/lib/FileSystem.jar";
    Framework-TraceModeling = "Ghidra/Debug/Framework-TraceModeling/lib/Framework-TraceModeling.jar";
    Generic = "Ghidra/Framework/Generic/lib/Generic.jar";
    Gui = "Ghidra/Framework/Gui/lib/Gui.jar";
    Help = "Ghidra/Framework/Help/lib/Help.jar";
    Project = "Ghidra/Framework/Project/lib/Project.jar";
    SoftwareModeling = "Ghidra/Framework/SoftwareModeling/lib/SoftwareModeling.jar";
    Utility = "Ghidra/Framework/Utility/lib/Utility.jar";
  };

  installGhidraJars =
    repoDir:
    lib.concatStringsSep "\n" (
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
    );

  pythonEnv = python3.withPackages (
    ps: with ps; [
      mcp
      requests
    ]
  );
in
maven.buildMavenPackage rec {
  pname = "ghidra-mcp";
  version = "5.14.2";

  src = fetchFromGitHub {
    owner = "bethington";
    repo = "ghidra-mcp";
    rev = "v${version}";
    hash = "sha256-2EMETCttJAz53GQaJDHtegb8+T2cHKmHZVMPrV5Cwxc=";
  };

  mvnJdk = jdk21;
  mvnParameters = "assembly:single";
  doCheck = false;

  postPatch = ''
    sed -i "s|<ghidra.version>[^<]*</ghidra.version>|<ghidra.version>${ghidraVersion}</ghidra.version>|" pom.xml
  '';

  mvnFetchExtraArgs.preBuild = installGhidraJars "$out/.m2";

  mvnHash = "sha256-MQbUB+EqZXs5jvALmEa/X9cdF/a+Ku5744+JWEQVS18=";

  nativeBuildInputs = [
    makeWrapper
    unzip
  ];

  installPhase = ''
    runHook preInstall

    mkdir -p $out/lib/ghidra/Ghidra/Extensions
    unzip -d $out/lib/ghidra/Ghidra/Extensions target/*.zip
    install -Dm644 bridge_mcp_ghidra.py $out/share/ghidra-mcp/bridge_mcp_ghidra.py
    makeWrapper ${pythonEnv}/bin/python $out/bin/bridge_mcp_ghidra \
      --add-flags "$out/share/ghidra-mcp/bridge_mcp_ghidra.py"

    runHook postInstall
  '';

  meta = {
    description = "MCP server + Ghidra plugin for AI-powered reverse engineering";
    mainProgram = "bridge_mcp_ghidra";
    homepage = "https://github.com/bethington/ghidra-mcp";
    license = lib.licenses.asl20;
    platforms = lib.platforms.unix;
  };
}
