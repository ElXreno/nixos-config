{
  config,
  namespace,
  lib,
  pkgs,
  ...
}:

let
  inherit (lib) mkIf mkEnableOption;
  cfg = config.${namespace}.programs.zed-editor;
in
{
  options.${namespace}.programs.zed-editor = {
    enable = mkEnableOption "Whether or not to manage Zed Editor.";
  };

  config = mkIf cfg.enable {
    programs = {
      zed-editor = {
        enable = true;
        extensions = [
          "nix"
          "github-actions"
          "gitlab-ci-ls"
          "git-firefly"
          "toml"
          "crates-lsp"
          "opentofu"
          "dockerfile"
          "helm"

          "golangci-lint"
          "gosum"
          "go-snippets"
        ];
        extraPackages =
          with pkgs;
          [
            nixd
            nixfmt-rfc-style

            gitlab-ci-ls
            opentofu
            tofu-ls
            helm-ls
            (tree-sitter.withPlugins (p: builtins.attrValues p))

            gopls
            golangci-lint-langserver
            delve
          ]
          ++ (with pkgs.${namespace}; [
            crates-lsp
          ]);

        userSettings = {
          telemetry = {
            diagnostics = false;
            metrics = false;
          };
          disable_ai = true;

          theme = "Ayu Dark";

          ensure_final_newline_on_save = true;
          load_direnv = "shell_hook";
          formatter = "language_server";

          languages = {
            Nix = {
              language_servers = [
                "nixd"
                "!nil"
              ];
            };
          };

          inlay_hints.enabled = true;
          lsp = {
            nixd = {
              initialization_options = {
                formatting = {
                  command = [
                    "nixfmt"
                    "--quiet"
                    "--"
                  ];
                };
              };
            };
          };
        };
      };
    };
  };
}
