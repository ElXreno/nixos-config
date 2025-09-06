{
  home-manager.users.elxreno.programs.ssh = {
    enable = true;
    enableDefaultConfig = false;
    matchBlocks = {
      "*" = {
        compression = true;
        serverAliveInterval = 10;
        serverAliveCountMax = 3;
        controlMaster = "auto";
        controlPath = "~/.ssh/master-%r@%n:%p";
        controlPersist = "yes";
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
}
