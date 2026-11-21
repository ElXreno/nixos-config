{
  lib,
  buildGoModule,
  gopls,
}:

buildGoModule {
  pname = "gopls-scope";
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

  nativeCheckInputs = [ gopls ];

  preCheck = ''
    export GOPLS=${lib.getExe gopls}
  '';

  meta = {
    description = "gopls proxy that scopes diagnostics to the modules Claude Code is editing";
    license = lib.licenses.mit;
    platforms = lib.platforms.linux;
    mainProgram = "gopls-scope";
  };
}
