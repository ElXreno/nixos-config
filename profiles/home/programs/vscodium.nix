{ pkgs, ... }:
{
  home-manager.users.elxreno.programs.vscode = {
    enable = true;
    # package = pkgs.vscodium;
    package = pkgs.vscode;
    profiles.default = {
      extensions =
        with pkgs.vscode-extensions;
        [
          jnoortheen.nix-ide
          github.copilot
          # hashicorp.terraform
          redhat.vscode-yaml
          # jebbs.plantuml
        ]
        ++ pkgs.vscode-utils.extensionsFromVscodeMarketplace [
          {
            name = "opentofu";
            publisher = "gamunu";
            version = "0.2.1";
            sha256 = "sha256-cZVKsdy92zlge3PJqVd7apzUKRaLPX10QUjQgv7V50M=";
            # hack due arm64 package by default
            arch = "linux-x64";

            postInstall = ''
              cd "$out/$installPrefix"
              ${pkgs.jq}/bin/jq '.contributes.configuration[2].properties."opentofu.languageServer.opentofu.path".default = "${pkgs.opentofu}/bin/tofu"' package.json | ${pkgs.moreutils}/bin/sponge package.json
            '';
          }
          {
            name = "logcat-color";
            publisher = "RaidXu";
            version = "0.0.1";
            sha256 = "sha256-sgiRl6iHPVu0S877qVjTcHVxkmckYm5kj6s0h8ikB4E=";
          }
          {
            name = "select-part-of-word";
            publisher = "mlewand";
            version = "1.0.1";
            sha256 = "sha256-2SBkvvrEZiJ47GQkd4bF6fVijSYGXDoIP8R+1/dVCWc=";
          }
        ];
      userSettings = {
        "editor.cursorSmoothCaretAnimation" = "on";
        "editor.fontSize" = 15;
        "editor.inlineSuggest.enabled" = true;
        "editor.minimap.enabled" = false;
        "editor.smoothScrolling" = true;
        "extensions.autoUpdate" = false;
        "extensions.ignoreRecommendations" = true;
        "files.associations" = {
          "logcat" = "logcat";
        };
        "git.autoStash" = true;
        "git.confirmSync" = false;
        "git.enableSmartCommit" = true;
        "git.useCommitInputAsStashMessage" = true;
        "github.copilot.enable" = {
          "*" = true;
        };
        "nix.enableLanguageServer" = true;
        "nix.formatterPath" = "nixfmt";
        "nix.serverPath" = "${pkgs.nixd}/bin/nixd";
        "security.workspace.trust.untrustedFiles" = "open";
        "telemetry.telemetryLevel" = "off";
        "update.mode" = "manual";
        "workbench.colorTheme" = "Default Light Modern";
      };
    };
    mutableExtensionsDir = false;
  };
  services.gnome.gnome-keyring.enable = true;

  home-manager.users.elxreno.xdg.mimeApps.defaultApplications = {
    # Don't abuse me by using LibreOffice or Thunderbird by default
    "text/plain" = "code.desktop";
    "text/xml" = "code.desktop";
  };
}
