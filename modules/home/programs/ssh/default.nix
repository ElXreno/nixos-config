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
      matchBlocks = {
        "*" = {
          serverAliveInterval = 10;
          serverAliveCountMax = 3;
        };
        "desktop" = {
          hostname = "100.107.189.129";
        };
        "biba" = {
          hostname = "109.171.24.112";
        };
        "boba" = {
          hostname = "109.171.24.112";
          port = 23;
        };

        "nixbuild" = {
          hostname = "eu.nixbuild.net";
          identityFile = "~/.ssh/nixbuild";
          identitiesOnly = true;
        };

        "fedoraproject" = {
          host = "*.fedoraproject.org *.fedorahosted.org *.fedorainfracloud.org fedorapeople.org";
          identityFile = "~/.ssh/id_rsa";
        };

        "git" = {
          host = "github.com gitlab.com gerrit.aospa.co aur.archlinux.org gitlab.tenders-ai.ru";
          identityFile = "~/.ssh/id_ecdsa-git";
        };
      };
    };
  };
}
