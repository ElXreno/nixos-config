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
  vendorHash = "sha256-VhV5NBtMhEAnEh8gdI8sDAn5WFOlffikGIvEwl8p4xs=";

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
