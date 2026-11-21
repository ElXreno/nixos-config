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
  cfg = config.${namespace}.users.alena;
in
{
  options.${namespace}.users.alena = {
    enable = mkEnableOption "configuring Alena user" // {
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
