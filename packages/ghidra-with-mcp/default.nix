{ pkgs, namespace }:
let
  inherit (pkgs.${namespace}) ghidra-mcp;
  ghidra-with-mcp = pkgs.ghidra.withExtensions (_: [ ghidra-mcp ]);
in
ghidra-with-mcp
