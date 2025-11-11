{ namespace, ... }:
{
  ${namespace} = {
    roles = {
      desktop.enable = true;
    };

    common-packages.enable = true;

    desktop-environments.plasma = {
      enable = true;
      wayland = false;
    };
  };

  home.stateVersion = "25.05";
}
