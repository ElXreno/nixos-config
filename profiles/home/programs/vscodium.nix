{ pkgs, ... }:
{
  home-manager.users.elxreno.programs.vscode = {
    enable = true;
    # package = pkgs.vscodium;
    package = pkgs.vscode;
    extensions = with pkgs.vscode-extensions; [
      jnoortheen.nix-ide
      github.copilot
    ] ++ pkgs.vscode-utils.extensionsFromVscodeMarketplace [
      {
        name = "logcat-color";
        publisher = "RaidXu";
        version = "0.0.1";
        sha256 = "sha256-sgiRl6iHPVu0S877qVjTcHVxkmckYm5kj6s0h8ikB4E=";
      }
    ];
    userSettings = {
      "editor.cursorSmoothCaretAnimation" = true;
      "editor.inlineSuggest.enabled" = true;
      "editor.minimap.enabled" = false;
      "editor.smoothScrolling" = true;
      "extensions.autoUpdate" = false;
      "extensions.ignoreRecommendations" = true;
      "files.associations" = { "logcat" = "logcat"; };
      "github.copilot.enable" = { "*" = true; };
      "nix.enableLanguageServer" = true;
      "nix.formatterPath" = "${pkgs.nixpkgs-fmt}/bin/nixpkgs-fmt";
      "nix.serverPath" = "${pkgs.rnix-lsp}/bin/rnix-lsp";
      "security.workspace.trust.untrustedFiles" = "open";
      "telemetry.telemetryLevel" = "off";
      "update.mode" = "manual";
      "editor.fontSize" = 15;
    };
    # Work-around: https://github.com/nix-community/home-manager/issues/2798#issuecomment-1073165352
    mutableExtensionsDir = false;
  };
  services.gnome.gnome-keyring.enable = true;

  home-manager.users.elxreno.xdg.mimeApps.defaultApplications = {
    # Don't abuse me by using LibreOffice or Thunderbird by default
    "text/plain" = "code.desktop";
    "text/xml" = "code.desktop";
  };
}
