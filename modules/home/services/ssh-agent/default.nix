{
  config,
  namespace,
  lib,
  pkgs,
  ...
}:
let
  inherit (lib) getExe mkIf mkEnableOption;
  cfg = config.${namespace}.services.ssh-agent;
in
{
  options.${namespace}.services.ssh-agent = {
    enable = mkEnableOption "managing ssh-agent";
  };

  config = mkIf cfg.enable {
    services.ssh-agent.enable = true;

    programs.ssh.settings."*".AddKeysToAgent = "yes";

    home.sessionVariables.SSH_ASKPASS = getExe pkgs.kdePackages.ksshaskpass;
  };
}
