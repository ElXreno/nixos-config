{
  config,
  namespace,
  lib,
  pkgs,
  ...
}:

let
  inherit (lib) mkIf mkEnableOption;
  cfg = config.${namespace}.roles.laptop;
in
{
  options.${namespace}.roles.laptop = {
    enable = mkEnableOption "Whether or not to enable laptop configuration.";
  };

  config = mkIf cfg.enable {
    ${namespace} = {
      roles.common.enable = true;

      programs = {
        firefox.enable = true;
        mpv.enable = true;
        zed-editor.enable = true;
      };
    };

    # Extra default home stuff
    home = {
      sessionPath = [ "/home/${config.home.username}/bin" ];
      sessionVariables = {
        _JAVA_OPTIONS = "-Dawt.useSystemAAFontSettings=lcd";
        DOTNET_CLI_TELEMETRY_OPTOUT = 1;
        NIXPKGS = "/home/${config.home.username}/projects/repos/github.com/NixOS/nixpkgs";
        GOROOT = "${pkgs.go}/share/go";
        GOPATH = "/home/${config.home.username}/.go";
      };
    };
  };
}
