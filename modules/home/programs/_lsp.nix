{ pkgs, lib }:
let
  bashLanguageServer = pkgs.writeShellScriptBin "bash-language-server" ''
    export PATH=${lib.makeBinPath [ pkgs.shellcheck ]}:$PATH
    exec ${lib.getExe pkgs.bash-language-server} "$@"
  '';

  servers = {
    bash = {
      package = bashLanguageServer;
      args = [ "start" ];
      extensions = {
        ".sh" = "shellscript";
        ".bash" = "shellscript";
      };
    };
    go = {
      package = pkgs.gopls;
      extensions.".go" = "go";
    };
    json = {
      package = pkgs.vscode-json-languageserver;
      args = [ "--stdio" ];
      extensions.".json" = "json";
    };
    nix = {
      package = pkgs.nixd;
      extensions.".nix" = "nix";
    };
    opentofu = {
      package = pkgs.tofu-ls;
      args = [ "serve" ];
      extensions = {
        ".tf" = "terraform";
        ".tfvars" = "terraform-vars";
      };
    };
    yaml = {
      package = pkgs.yaml-language-server;
      args = [ "--stdio" ];
      extensions = {
        ".yaml" = "yaml";
        ".yml" = "yaml";
      };
    };
    helm = {
      package = pkgs.helm-ls;
      args = [ "serve" ];
      extensions.".tpl" = "helm";
    };
  };
in
{
  inherit servers;

  packages = lib.mapAttrsToList (_: server: server.package) servers ++ [ pkgs.nixfmt ];
}
