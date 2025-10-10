{
  lib,
  namespace,
  ...
}:

{
  imports = [ (lib.snowfall.fs.get-file "systems/x86_64-linux/DESTROYER/default.nix") ];

  ${namespace} = {
    services = {
      atticd.enable = lib.mkForce false;
      xray.server.enable = lib.mkForce false;
    };
  };

  virtualisation = {
    cores = 4;
    memorySize = 4096;
    diskImage = null;
  };
}
