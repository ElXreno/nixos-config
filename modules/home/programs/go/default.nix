{
  config,
  namespace,
  lib,
  ...
}:
let
  inherit (lib) mkIf mkEnableOption;
  cfg = config.${namespace}.programs.go;
in
{
  options.${namespace}.programs.go = {
    enable = mkEnableOption "managing the Go toolchain";
  };

  config = mkIf cfg.enable {
    programs.go = {
      enable = true;
      env.GOPATH = "${config.home.homeDirectory}/.go";
      telemetry.mode = "off";
    };
  };
}
