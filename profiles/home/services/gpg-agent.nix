{ pkgs, ... }: {
  home-manager.users.elxreno.services.gpg-agent = {
    enable = true;
    enableSshSupport = true;
    defaultCacheTtl = 300;
    pinentryPackage = pkgs.pinentry-qt;
  };
}
