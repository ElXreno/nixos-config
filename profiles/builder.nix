{
  users.users.builder = {
    isNormalUser = true;
    openssh.authorizedKeys.keys = [
      "ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAAIHYilJ1YCds1/4Gqli2wNyhIW/cUyMPMXe2Vv1xkbG/u hydra"
    ];
  };

  nix.settings.trusted-users = [ "builder" ];
}
