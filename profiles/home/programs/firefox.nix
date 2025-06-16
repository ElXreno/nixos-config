{ pkgs, ... }:
{
  home-manager.users.elxreno.programs.firefox = {
    enable = true;
    package = pkgs.firefox;
  };

  home-manager.users.elxreno.xdg.mimeApps.defaultApplications = {
    # Don't abuse me by using Thunderbird by default
    "text/html" = "firefox.desktop";
    "x-scheme-handler/http" = "firefox.desktop";
    "x-scheme-handler/https" = "firefox.desktop";
    "x-scheme-handler/about" = "firefox.desktop";
    "x-scheme-handler/unknown" = "firefox.desktop";
  };
}
