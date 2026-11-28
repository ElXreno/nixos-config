{
  lib,
  buildGoModule,
}:

buildGoModule {
  pname = "model-scan";
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
    description = "Static scanner for downloaded models: GGUF and safetensors payloads, poisoned chat templates, pickle imports";
    license = lib.licenses.mit;
    platforms = lib.platforms.linux;
    mainProgram = "model-scan";
  };
}
