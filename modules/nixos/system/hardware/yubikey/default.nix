{
  config,
  namespace,
  lib,
  pkgs,
  ...
}:
let
  inherit (lib) mkIf mkEnableOption;
  cfg = config.${namespace}.system.hardware.yubikey;
in
{
  options.${namespace}.system.hardware.yubikey = {
    enable = mkEnableOption "YubiKey support";
  };

  config = mkIf cfg.enable {
    services.pcscd.enable = true;
    environment.systemPackages = [ pkgs.yubikey-manager ];

    programs.yubikey-touch-detector.enable = true;

    security.pam = {
      u2f.settings = {
        cue = true;
        origin = "pam://elxreno-clan";
        appid = "pam://elxreno-clan";
        authfile = pkgs.writeText "u2f-mappings" (builtins.readFile ./u2f-mappings);
      };
      services = {
        sudo.u2f.enable = true;
        login.u2f.enable = true;
      };
    };
  };
}
