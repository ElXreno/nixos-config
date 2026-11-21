{
  config,
  osConfig,
  namespace,
  lib,
  ...
}:

let
  inherit (lib) mkIf mkEnableOption;
  cfg = config.${namespace}.roles.laptop;
in
{
  options.${namespace}.roles.laptop = {
    enable = mkEnableOption "laptop configuration" // {
      default = osConfig.${namespace}.roles.laptop.enable;
    };
  };

  config = mkIf cfg.enable {
    ${namespace} = {
      roles.common.enable = true;

      programs = {
        firefox.enable = true;
        mpv.enable = true;
      };
    };

    # Extra default home stuff
    home = {
      sessionPath = [ "/home/${config.home.username}/bin" ];
      sessionVariables = {
        _JAVA_OPTIONS = "-Dawt.useSystemAAFontSettings=lcd";
      };
    };

    xdg = {
      mimeApps.enable = true;
      configFile."mimeapps.list".force = true;
    };
  };
}
