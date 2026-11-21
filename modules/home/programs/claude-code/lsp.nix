{
  config,
  namespace,
  lib,
  pkgs,
  ...
}:
let
  cfg = config.${namespace}.programs.claude-code;
  lsp = import ../_lsp.nix { inherit pkgs lib; };
in
{
  config = lib.mkIf cfg.enable {
    programs.claude-code.lspServers =
      lib.recursiveUpdate
        (lib.mapAttrs (
          _: server:
          {
            command = lib.getExe server.package;
            extensionToLanguage = server.extensions;
          }
          // lib.optionalAttrs (server ? args) { inherit (server) args; }
        ) lsp.servers)
        {
          go = {
            command = lib.getExe pkgs.${namespace}.gopls-scope;
            args = [ (lib.getExe pkgs.gopls) ];
          };
          python = {
            command = lib.getExe' pkgs.basedpyright "basedpyright-langserver";
            args = [ "--stdio" ];
            extensionToLanguage = {
              ".py" = "python";
              ".pyi" = "python";
            };
          };
          rust = {
            command = lib.getExe pkgs.rust-analyzer;
            extensionToLanguage = {
              ".rs" = "rust";
            };
          };
          toml = {
            command = lib.getExe pkgs.taplo;
            args = [
              "lsp"
              "stdio"
            ];
            extensionToLanguage = {
              ".toml" = "toml";
            };
          };
          typescript = {
            command = lib.getExe pkgs.typescript-language-server;
            args = [ "--stdio" ];
            extensionToLanguage = {
              ".ts" = "typescript";
              ".tsx" = "typescriptreact";
              ".js" = "javascript";
              ".jsx" = "javascriptreact";
            };
          };
          yaml.settings.yaml.schemas = {
            "https://raw.githubusercontent.com/fluxcd-community/flux2-schemas/refs/heads/main/all.json" = [
              "**/*helmrelease*.yaml"
              "**/*helmrepository*.yaml"
              "**/*gitrepository*.yaml"
              "**/*ocirepository*.yaml"
              "**/*.Kustomization*.yaml"
            ];
            "https://www.schemastore.org/kustomization.json" = [ "**/kustomization.yaml" ];
          };
        };
  };
}
