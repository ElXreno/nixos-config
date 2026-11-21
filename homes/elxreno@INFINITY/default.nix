{ namespace, ... }:
{
  programs.plasma.kwin.nightLight = {
    enable = true;
    mode = "automatic";
  };

  ${namespace}.services.syncthing.enable = true;
}
