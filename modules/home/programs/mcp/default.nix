{
  config,
  namespace,
  lib,
  pkgs,
  ...
}:
let
  inherit (lib) mkIf mkEnableOption;
  cfg = config.${namespace}.programs.mcp;
in
{
  options.${namespace}.programs.mcp = {
    enable = mkEnableOption "managing MCP servers";
  };

  config = mkIf cfg.enable {
    programs.mcp = {
      enable = true;
      servers.ghidra-mcp.command = "${lib.getExe pkgs.${namespace}.ghidra-mcp}";
    };
  };
}
