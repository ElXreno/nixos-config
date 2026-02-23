{ pkgs, namespace, ... }:
let
  ghidra-mcp = pkgs.${namespace}.ghidra-mcp;
  ghidra-with-mcp = pkgs.ghidra.withExtensions (_: [ ghidra-mcp ]);
in
pkgs.mkShell {
  packages = [
    ghidra-with-mcp
    ghidra-mcp # for bridge_mcp_ghidra binary
  ];

  shellHook = ''
    echo ""
    echo "ghidra-mcp devshell"
    echo "  Run Ghidra:     ghidra"
    echo "  Run MCP bridge: bridge_mcp_ghidra"
    echo ""
  '';
}
