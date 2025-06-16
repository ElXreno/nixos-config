{ pkgs, ... }:
{
  home-manager.users.elxreno.services.gpg-agent = {
    enable = true;
    enableSshSupport = true;
    defaultCacheTtl = 300;
    pinentry.package = pkgs.pinentry-qt;
  };
}
