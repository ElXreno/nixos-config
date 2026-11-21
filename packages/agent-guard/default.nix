{
  lib,
  buildGoModule,
  gitMinimal,
  openssh,
  tree-sitter-grammars,
  writeText,
}:

let
  grammars = lib.genAttrs [
    "bash"
    "c"
    "c-sharp"
    "cmake"
    "cpp"
    "dart"
    "dockerfile"
    "elixir"
    "elm"
    "fish"
    "gitignore"
    "go"
    "haskell"
    "hcl"
    "ini"
    "java"
    "javascript"
    "julia"
    "kotlin"
    "lua"
    "make"
    "nix"
    "perl"
    "php"
    "properties"
    "proto"
    "python"
    "qmljs"
    "r"
    "ruby"
    "rust"
    "scala"
    "sql"
    "swift"
    "toml"
    "tsx"
    "typescript"
    "yaml"
    "zig"
  ] (name: "${tree-sitter-grammars."tree-sitter-${name}"}/parser");

  grammarsFile = writeText "agent-guard-grammars.json" (builtins.toJSON grammars);

  testEnv = {
    AGENT_GUARD_GRAMMARS = "${grammarsFile}";
    AGENT_GUARD_REQUIRE_GRAMMARS = "1";
  };
in
buildGoModule {
  pname = "agent-guard";
  version = "0.2.0";

  src = lib.fileset.toSource {
    root = ./.;
    fileset = lib.fileset.unions [
      ./go.mod
      ./go.sum
      ./main.go
      ./internal
      ./pi
    ];
  };

  proxyVendor = true;
  vendorHash = "sha256-4WEVLSSj3QTQ9n7E31t2dyWaGFzFeIm3Lq6/BopbinI=";

  ldflags = [
    "-s"
    "-w"
    "-X agent-guard/internal/checks.gitProgram=${lib.getExe gitMinimal}"
    "-X agent-guard/internal/checks.sshProgram=${lib.getExe openssh}"
  ];

  env = testEnv;

  passthru = {
    inherit grammars grammarsFile testEnv;
    piExtension = ./pi/agent-guard.ts;
  };

  meta = {
    description = "Guard hooks for coding agent harnesses in one binary";
    license = lib.licenses.mit;
    platforms = lib.platforms.linux;
    mainProgram = "agent-guard";
  };
}
