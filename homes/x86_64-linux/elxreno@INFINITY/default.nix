{ namespace, ... }:
{
  ${namespace} = {
    roles = {
      laptop.enable = true;
    };

    desktop-environments.plasma.enable = true;

    services = {
      syncthing.enable = true;
    };
  };

  home.stateVersion = "25.05";
}
