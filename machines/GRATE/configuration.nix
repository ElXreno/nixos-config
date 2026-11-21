{
  namespace,
  pkgs,
  config,
  ...
}:
{
  ${namespace} = {
    system = {
      hardware.gpu.intel = {
        computeRuntime = pkgs.intel-compute-runtime-legacy1;
        loadHuc = true;
      };

      hardware.morefine.m6s.enable = true;

      networking.wireless = {
        enable = true;
        networks = {
          Home = { };
          Homelander.saeOnly = true;
        };
      };
    };

    services = {
      nixflix.enable = true;
      ripe-atlas.enable = true;

      home-assistant.enable = true;

      dns-sinkhole = {
        enable = true;
        interface = builtins.head config.${namespace}.facts.network.wifi.interfaces;
      };
    };
  };
}
