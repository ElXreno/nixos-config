{ namespace, ... }:
{
  ${namespace} = {
    roles = {
      server.enable = true;
    };
  };

  home.stateVersion = "25.05";
}
