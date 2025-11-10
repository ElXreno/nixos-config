{ namespace, ... }:
{
  ${namespace} = {
    roles.server.enable = true;

    programs.htop.showAdvancedCPUStats = true;
  };

  home.stateVersion = "25.05";
}
