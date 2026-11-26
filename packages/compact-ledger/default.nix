{
  lib,
  buildGoModule,
}:

buildGoModule {
  pname = "compact-ledger";
  version = "0.1.0";

  src = lib.fileset.toSource {
    root = ./.;
    fileset = lib.fileset.fileFilter (file: file.hasExt "go" || file.name == "go.mod") ./.;
  };

  vendorHash = null;

  ldflags = [
    "-s"
    "-w"
  ];

  meta = {
    description = "Claude Code compaction hooks that keep earlier summaries and inject them after each compaction";
    license = lib.licenses.mit;
    platforms = lib.platforms.linux;
    mainProgram = "compact-ledger";
  };
}
