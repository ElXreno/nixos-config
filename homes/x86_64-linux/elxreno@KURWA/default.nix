{ namespace, ... }:
{
  ${namespace} = {
    roles = {
      laptop.enable = true;
    };

    desktop-environments.hyprland.enable = true;

    common-packages.enable = true;

    programs = {
      gpg.enable = true;
      mangohud.enable = true;
      ssh.enable = true;
    };

    services = {
      gpg-agent.enable = true;
      syncthing.enable = true;
    };
  };

  home.stateVersion = "25.05";
}
