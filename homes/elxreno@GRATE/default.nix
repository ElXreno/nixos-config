{ namespace, ... }:
{
  ${namespace} = {
    roles.server.enable = true;

    programs.htop.showAdvancedCPUStats = true;

    services = {
      gpg-agent.enable = true;
    };
  };
}
