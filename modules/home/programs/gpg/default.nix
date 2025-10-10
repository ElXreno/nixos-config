{
  config,
  namespace,
  lib,
  ...
}:
let
  inherit (lib) mkIf mkEnableOption;
  cfg = config.${namespace}.programs.gpg;
in
{
  options.${namespace}.programs.gpg = {
    enable = mkEnableOption "Whether or not to manage gpg.";
  };

  config = mkIf cfg.enable {
    programs.gpg = {
      enable = true;
    };
  };
}
