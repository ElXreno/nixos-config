{ namespace, ... }:

{
  ${namespace} = {
    system = {
      hardware.gpu.intel = {
        vaapiDriver = "i965";
        computeRuntime = null;
      };
    };

    desktop-environments.niri.enable = true;
  };
}
