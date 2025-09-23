{
  pkgs,
  ...
}:
{
  home-manager.users.elxreno.programs = {
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
      extraPackages = with pkgs; [
        nixd
        nixfmt-rfc-style

        crates-lsp
        gitlab-ci-ls
        opentofu
        tofu-ls
        helm-ls
        (tree-sitter.withPlugins (p: builtins.attrValues p))

        gopls
        golangci-lint-langserver
        delve
      ];

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
}
