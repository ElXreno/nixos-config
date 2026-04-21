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
    enable = mkEnableOption "Whether to manage MCP servers.";
  };

  config = mkIf cfg.enable {
    programs.mcp = {
      enable = true;
      servers = {
        nixos.command = "${lib.getExe pkgs.mcp-nixos}";
        ghidra-mcp.command = "${lib.getExe pkgs.${namespace}.ghidra-mcp}";
        playwright.command = "${lib.getExe pkgs.playwright-mcp}";
      };
    };
  };
}
