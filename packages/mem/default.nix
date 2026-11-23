{
  lib,
  buildGoModule,
}:

buildGoModule {
  pname = "mem";
  version = "0.1.0";

  src = lib.fileset.toSource {
    root = ./.;
    fileset = lib.fileset.unions [
      ./go.mod
      ./go.sum
      ./main.go
      ./internal
    ];
  };

  proxyVendor = true;
  vendorHash = "sha256-AFIu2fb/a4T75T5T2Mtj4eg/q7Mwe948q40uHPxxFXs=";

  ldflags = [
    "-s"
    "-w"
  ];

  meta = {
    description = "Agent memory store with a duplicate gate, supersession and FTS5 recall";
    license = lib.licenses.mit;
    platforms = lib.platforms.linux;
    mainProgram = "mem";
  };
}
