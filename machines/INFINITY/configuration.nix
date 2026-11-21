{
  namespace,
  ...
}:

{
  ${namespace} = {
    system = {
      impermanence.enable = true;

      nix.gc.enable = true;
    };

    desktop-environments.plasma.enable = true;

    home-manager.syncthing.randomPortIncrement = 66;
  };
}
