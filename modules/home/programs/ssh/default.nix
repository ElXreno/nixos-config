{
  config,
  namespace,
  lib,
  ...
}:
let
  inherit (lib) mkIf mkEnableOption;
  cfg = config.${namespace}.programs.ssh;
in
{
  options.${namespace}.programs.ssh = {
    enable = mkEnableOption "Whether or not to manage ssh.";
  };

  config = mkIf cfg.enable {
    programs.ssh = {
      enable = true;
      enableDefaultConfig = false;
      settings = {
        "*" = {
          ServerAliveInterval = 10;
          ServerAliveCountMax = 3;
          SetEnv.TERM = "xterm-256color";
        };
        "biba" = {
          HostName = "109.171.24.112";
        };
        "boba" = {
          HostName = "109.171.24.112";
          Port = 23;
        };

        "nixbuild" = {
          HostName = "eu.nixbuild.net";
          IdentityFile = "~/.ssh/nixbuild";
          IdentitiesOnly = true;
        };

        "*.fedoraproject.org *.fedorahosted.org *.fedorainfracloud.org fedorapeople.org" = {
          IdentityFile = "~/.ssh/id_rsa";
        };

        "github.com gitlab.com gerrit.aospa.co aur.archlinux.org gitlab.tenders-ai.ru" = {
          IdentityFile = "~/.ssh/id_ecdsa-git";
        };
      };
    };
  };
}
