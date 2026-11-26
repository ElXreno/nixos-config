{
  lib,
  maven,
  ghidra,
  python3Packages,
  unzip,
  openjdk25,
  fetchFromGitHub,
  nix-update-script,
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
    Graph = "Ghidra/Framework/Graph/lib/Graph.jar";
    Gui = "Ghidra/Framework/Gui/lib/Gui.jar";
    Help = "Ghidra/Framework/Help/lib/Help.jar";
    Project = "Ghidra/Framework/Project/lib/Project.jar";
    SoftwareModeling = "Ghidra/Framework/SoftwareModeling/lib/SoftwareModeling.jar";
    Utility = "Ghidra/Framework/Utility/lib/Utility.jar";
  };

  installGhidraJars =
    mvnFlags: repoDir:
    lib.concatStringsSep "\n" (
      lib.mapAttrsToList (name: jarPath: ''
        mvn -q ${mvnFlags} install:install-file \
          -Dmaven.repo.local="${repoDir}" \
          -Dfile="${ghidraHome}/${jarPath}" \
          -DgroupId=ghidra \
          -DartifactId=${name} \
          -Dversion=${ghidraVersion} \
          -Dpackaging=jar \
          -DgeneratePom=true
      '') ghidraJars
    );

  version = "7.1.0";

  src = fetchFromGitHub {
    owner = "bethington";
    repo = "ghidra-mcp";
    rev = "v${version}";
    hash = "sha256-4xWLBpdkktFCf2yI4bHDqpE+jXqZ8k8QdPM9Nh0Yf1w=";
  };

  pythonPackages = python3Packages.overrideScope (final: _prev: { mcp = final.mcp_1; });

  bridge = pythonPackages.buildPythonApplication {
    pname = "ghidra-mcp-bridge";
    inherit version src;
    pyproject = true;

    build-system = [ pythonPackages.hatchling ];
    dependencies = [
      pythonPackages.mcp
      pythonPackages.pydantic
    ];

    pythonImportsCheck = [ "bridge_mcp_ghidra" ];
  };
in
maven.buildMavenPackage {
  pname = "ghidra-mcp";
  inherit version src;

  mvnJdk = openjdk25;
  mvnParameters = "-Dghidra.version=${ghidraVersion} assembly:single";
  doCheck = false;

  mvnFetchExtraArgs = {
    preBuild = installGhidraJars "" "$out/.m2";
    postInstall = ''
      rm -rf $out/.m2/ghidra
    '';
  };

  afterDepsSetup = installGhidraJars "-o" "$mvnDeps/.m2";

  mvnHash = "sha256-bwwQze6+saD7OLqGviEoQgwuDN8hTxNHRaS26YbsXew=";

  nativeBuildInputs = [ unzip ];

  installPhase = ''
    runHook preInstall

    mkdir -p $out/lib/ghidra/Ghidra/Extensions $out/bin
    unzip -d $out/lib/ghidra/Ghidra/Extensions target/*.zip
    ln -s ${bridge}/bin/bridge-mcp-ghidra $out/bin/bridge-mcp-ghidra

    runHook postInstall
  '';

  passthru = {
    inherit bridge;
    updateScript = nix-update-script { };
  };

  meta = {
    description = "MCP server + Ghidra plugin for AI-powered reverse engineering";
    mainProgram = "bridge-mcp-ghidra";
    homepage = "https://github.com/bethington/ghidra-mcp";
    license = lib.licenses.asl20;
    platforms = lib.platforms.unix;
  };
}
