{
  config,
  namespace,
  lib,
  ...
}:

let
  inherit (lib)
    mkIf
    mkEnableOption
    ;
  cfg = config.${namespace}.user.alena;
in
{
  options.${namespace}.user.alena = {
    enable = mkEnableOption "Whether to configure Alena user." // {
      default = config.home.username == "alena";
    };
  };

  config = mkIf cfg.enable {
    ${namespace}.programs.firefox = {
      settings = {
        "intl.locale.requested" = "ru,en-US";
      };
    };
  };
}
