{
  home-manager.users.elxreno.programs.ssh = {
    enable = true;
    # compression = true;
    serverAliveInterval = 10;
    matchBlocks = {
      "desktop" = {
        hostname = "10.10.10.30";
      };
      "biba" = {
        hostname = "109.171.24.112";
      };
      "boba" = {
        hostname = "109.171.24.112";
        port = 23;
      };

      "fedoraproject" = {
        host = "*.fedoraproject.org *.fedorahosted.org *.fedorainfracloud.org fedorapeople.org";
        identityFile = "~/.ssh/id_rsa";
      };

      "git" = {
        host = "github.com gitlab.com gerrit.aospa.co aur.archlinux.org code.elxreno.ninja";
        identityFile = "~/.ssh/id_ecdsa-git";
      };
    };
  };
}
