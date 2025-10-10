{
  config,
  namespace,
  lib,
  ...
}:
let
  inherit (lib)
    mkEnableOption
    ;
  cfg = config.${namespace}.system.documentation;
in
{
  options.${namespace}.system.documentation = {
    enable = mkEnableOption "Whether to install documentation." // {
      default = true;
    };
  };

  config = {
    documentation = {
      enable = cfg.enable;
      nixos.enable = cfg.enable;
      man.enable = cfg.enable;
      info.enable = cfg.enable;
      doc.enable = cfg.enable;
    };
  };
}
