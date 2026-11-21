{
  config,
  osConfig,
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
    enable = mkEnableOption "managing gpg-agent";
  };

  config = mkIf cfg.enable {
    programs.gpg.scdaemonSettings = mkIf osConfig.${namespace}.system.hardware.yubikey.enable {
      disable-ccid = true;
    };

    services.gpg-agent = {
      enable = true;
      defaultCacheTtl = 300;
      pinentry.package = pkgs.pinentry-qt;
    };
  };
}
