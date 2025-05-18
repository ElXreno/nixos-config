{
  home-manager.users.elxreno.programs.ssh = {
    enable = true;
    # compression = true;
    serverAliveInterval = 10;
    matchBlocks = {
      "desktop" = { hostname = "100.107.189.129"; };
      "biba" = { hostname = "109.171.24.112"; };
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
        host =
          "*.fedoraproject.org *.fedorahosted.org *.fedorainfracloud.org fedorapeople.org";
        identityFile = "~/.ssh/id_rsa";
      };

      "git" = {
        host =
          "github.com gitlab.com gerrit.aospa.co aur.archlinux.org gitlab-ssh.angora-ide.ts.net";
        identityFile = "~/.ssh/id_ecdsa-git";
      };
    };
  };
}
