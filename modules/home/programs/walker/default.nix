{
  config,
  namespace,
  lib,
  pkgs,
  ...
}:

let
  inherit (lib)
    mkIf
    mkEnableOption
    ;
  cfg = config.${namespace}.programs.walker;
in
{
  options.${namespace}.programs.walker = {
    enable = mkEnableOption "Whether or not to manage walker.";
  };

  config = mkIf cfg.enable {
    programs.walker = {
      enable = true;
      runAsService = true;

      elephant = {
        provider.desktopapplications.settings.launch_prefix = "${lib.getExe pkgs.app2unit} --";
        provider.websearch.settings = {
          always_show_default = false;
          entries = [
            {
              name = "Google";
              url = "https://www.google.com/search?q=%TERM%";
              default = true;
            }
            {
              name = "Wikipedia (w:)";
              url = "https://en.wikipedia.org/wiki/Special:Search?search=%TERM%";
              prefix = "w:";
            }
            {
              name = "NixOS Options (no:)";
              url = "https://search.nixos.org/options?query=%TERM%&channel=unstable";
              prefix = "no:";
            }
            {
              name = "Home Manager Options (hm:)";
              url = "https://home-manager-options.extranix.com/?query=%TERM%&release=master";
              prefix = "hm:";
            }
          ];
        };
      };

      config = {
        providers = {
          default = [
            "desktopapplications"
            "calc"
            "windows"
          ];
        };
      };
    };
  };
}
