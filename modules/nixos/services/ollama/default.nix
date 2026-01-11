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
    mkPackageOption
    ;
  cfg = config.${namespace}.services.ollama;
in
{
  options.${namespace}.services.ollama = {
    enable = mkEnableOption "Whether or not to manage Ollama.";
    package = mkPackageOption pkgs "ollama-vulkan" { };
  };

  config = mkIf cfg.enable {
    services.ollama = {
      enable = true;
      inherit (cfg) acceleration package;
    };
  };
}
