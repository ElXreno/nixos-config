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
            yaml-language-server
            (runCommand "json-language-server" { } ''
              mkdir -p "$out"/bin
              ln -s ${lib.getExe nodePackages.vscode-json-languageserver} "$out"/bin/json-language-server
              ln -s ${lib.getExe nodePackages.vscode-json-languageserver} "$out"/bin/vscode-json-languageserver
            '')
            (tree-sitter.withPlugins (p: builtins.attrValues p))

            gopls
            golangci-lint-langserver
            delve
            package-version-server
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

          file_types = {
            Helm = [
              "**/templates/**/*.tpl"
              "**/templates/**/*.yaml"
              "**/templates/**/*.yml"
              "**/helmfile.d/**/*.yaml"
              "**/helmfile.d/**/*.yml"
            ];
          };

          languages = {
            Nix = {
              language_servers = [
                "nixd"
                "!nil"
              ];
            };
            helm_ls = {
              settings = {
                helm-ls = {
                  logLevel = "info";
                  yamlls = {
                    enabled = true;
                  };
                };
              };
            };
            yamlls = {
              initialization_options = {
                yaml = {
                  schemas = {
                    kubernetes = "templates/*.yaml";
                    "http://json.schemastore.org/github-workflow" = ".github/workflows/*";
                    "http://json.schemastore.org/github-action" = ".github/action.{yml,yaml}";
                    "http://json.schemastore.org/ansible-stable-2.9" = "roles/tasks/*.{yml,yaml}";
                    "http://json.schemastore.org/prettierrc" = ".prettierrc.{yml,yaml}";
                    "http://json.schemastore.org/kustomization" = "kustomization.{yml,yaml}";
                    "http://json.schemastore.org/ansible-playbook" = "*play*.{yml,yaml}";
                    "http://json.schemastore.org/chart" = "Chart.{yml,yaml}";
                    "https://json.schemastore.org/dependabot-v2" = ".github/dependabot.{yml,yaml}";
                    "https://json.schemastore.org/gitlab-ci" = "*gitlab-ci*.{yml,yaml}";
                    "https://raw.githubusercontent.com/OAI/OpenAPI-Specification/main/schemas/v3.1/schema.json" =
                      "*api*.{yml,yaml}";
                    "https://raw.githubusercontent.com/compose-spec/compose-spec/master/schema/compose-spec.json" =
                      "*docker-compose*.{yml,yaml}";
                    "https://raw.githubusercontent.com/argoproj/argo-workflows/master/api/jsonschema/schema.json" =
                      "*flow*.{yml,yaml}";
                  };
                };
              };
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
