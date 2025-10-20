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
    mkOption
    types
    optionalAttrs
    ;
  cfg = config.${namespace}.services.ollama;
in
{
  options.${namespace}.services.ollama = {
    enable = mkEnableOption "Whether or not to manage Ollama.";
    acceleration = mkOption {
      type = types.nullOr (
        types.enum [
          false
          "rocm"
          "cuda"
        ]
      );
      default = false;
    };
  };

  config = mkIf cfg.enable {
    services.ollama = {
      enable = true;
      acceleration = cfg.acceleration;
    }
    // optionalAttrs (cfg.acceleration != false) {
      rocmOverrideGfx = "11.0.1";
      package = pkgs.ollama-rocm-igpu;
    };
  };
}
