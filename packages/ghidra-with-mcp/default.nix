{ pkgs, namespace }:
let
  inherit (pkgs) symlinkJoin makeWrapper;
  inherit (pkgs.${namespace}) ghidra-mcp ghidra-jieli;
  base = pkgs.ghidra.withExtensions (_: [
    ghidra-mcp
    ghidra-jieli
  ]);
in
symlinkJoin {
  name = "ghidra-with-mcp-${pkgs.ghidra.version}";
  paths = [ base ];
  nativeBuildInputs = [ makeWrapper ];
  postBuild = ''
    for bin in $out/bin/*; do
      wrapProgram "$bin" --set GHIDRA_MCP_ALLOW_SCRIPTS 1
    done
  '';
  meta = pkgs.ghidra.meta // {
    description = "Ghidra with the GhidraMCP extension preinstalled";
  };
}
