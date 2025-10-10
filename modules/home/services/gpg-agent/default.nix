{
  config,
  namespace,
  lib,
  pkgs,
  ...
}:

let
  inherit (lib) mkIf mkEnableOption;
  cfg = config.${namespace}.services.gpg-agent;
in
{
  options.${namespace}.services.gpg-agent = {
    enable = mkEnableOption "Whether or not to manage gpg-agent.";
  };

  config = mkIf cfg.enable {
    services.gpg-agent = {
      enable = true;
      enableSshSupport = true;
      defaultCacheTtl = 300;
      pinentry.package = pkgs.pinentry-qt;
    };
  };
}
