{
  lib,
  buildGoModule,
}:

buildGoModule {
  pname = "rewalk";
  version = "0.1.0";

  src = lib.fileset.toSource {
    root = ./.;
    fileset = lib.fileset.fileFilter (file: file.hasExt "go" || file.name == "go.mod") ./.;
  };

  vendorHash = null;

  subPackages = [
    "."
    "cmd/orch-tool"
  ];

  ldflags = [
    "-s"
    "-w"
  ];

  postInstall = ''
    mv $out/bin/orch $out/bin/rewalk
  '';

  meta = {
    description = "Stand that counts how often a coding agent goes back to branches it has already closed";
    license = lib.licenses.mit;
    platforms = lib.platforms.linux;
    mainProgram = "rewalk";
  };
}
