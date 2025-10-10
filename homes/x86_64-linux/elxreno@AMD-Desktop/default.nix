{ namespace, ... }:
{
  ${namespace} = {
    roles = {
      desktop.enable = true;
    };

    desktop-environments.plasma.enable = true;
  };

  home.stateVersion = "25.05";
}
