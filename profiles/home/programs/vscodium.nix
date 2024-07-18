{ pkgs, ... }: {
  home-manager.users.elxreno.programs.vscode = {
    enable = true;
    # package = pkgs.vscodium;
    package = pkgs.vscode;
    extensions = with pkgs.vscode-extensions;
      [
        jnoortheen.nix-ide
        github.copilot
        hashicorp.terraform
        redhat.vscode-yaml
        jebbs.plantuml
      ] ++ pkgs.vscode-utils.extensionsFromVscodeMarketplace [{
        name = "logcat-color";
        publisher = "RaidXu";
        version = "0.0.1";
        sha256 = "sha256-sgiRl6iHPVu0S877qVjTcHVxkmckYm5kj6s0h8ikB4E=";
      }];
    userSettings = {
      "editor.cursorSmoothCaretAnimation" = "on";
      "editor.fontSize" = 15;
      "editor.inlineSuggest.enabled" = true;
      "editor.minimap.enabled" = false;
      "editor.smoothScrolling" = true;
      "extensions.autoUpdate" = false;
      "extensions.ignoreRecommendations" = true;
      "files.associations" = { "logcat" = "logcat"; };
      "git.autoStash" = true;
      "git.confirmSync" = false;
      "git.enableSmartCommit" = true;
      "git.useCommitInputAsStashMessage" = true;
      "github.copilot.enable" = { "*" = true; };
      "nix.enableLanguageServer" = true;
      "nix.formatterPath" = "${pkgs.nixpkgs-fmt}/bin/nixpkgs-fmt";
      "nix.serverPath" = "${pkgs.nil}/bin/nil";
      "security.workspace.trust.untrustedFiles" = "open";
      "telemetry.telemetryLevel" = "off";
      "update.mode" = "manual";
      "workbench.colorTheme" = "Default Light Modern";
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
