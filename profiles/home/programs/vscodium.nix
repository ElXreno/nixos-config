{ pkgs, ... }:
{
  home-manager.users.elxreno.programs.vscode = {
    enable = true;
    package = pkgs.vscodium;
    # package = pkgs.vscode;
    profiles.default = {
      extensions =
        with pkgs.vscode-extensions;
        [
          jnoortheen.nix-ide
          redhat.vscode-yaml
          jebbs.plantuml
          # continue.continue
        ]
        ++ pkgs.vscode-utils.extensionsFromVscodeMarketplace [
          {
            name = "vscode-opentofu";
            publisher = "OpenTofu";
            version = "0.3.3";
            sha256 = "sha256-4142LtuWpWhAZqklHjMyZuFoTrGwRIqXxGjC+xBn5sc=";

            postInstall = ''
              cd "$out/$installPrefix"
              ${pkgs.jq}/bin/jq '.contributes.configuration[1].properties."opentofu.languageServer.path".default = "${pkgs.tofu-ls}/bin/tofu-ls"' package.json | ${pkgs.moreutils}/bin/sponge package.json
              ${pkgs.jq}/bin/jq '.contributes.configuration[2].properties."opentofu.languageServer.tofu.path".default = "${pkgs.opentofu}/bin/tofu"' package.json | ${pkgs.moreutils}/bin/sponge package.json
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
          {
            name = "aw-watcher-vscode";
            publisher = "activitywatch";
            version = "0.5.0";
            sha256 = "sha256-OrdIhgNXpEbLXYVJAx/jpt2c6Qa5jf8FNxqrbu5FfFs=";
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
        "git.autofetch" = true;
        "git.autoStash" = true;
        "git.confirmSync" = false;
        "git.enableSmartCommit" = true;
        "git.useCommitInputAsStashMessage" = true;
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
