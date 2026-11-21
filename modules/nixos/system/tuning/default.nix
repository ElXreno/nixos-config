{
  namespace,
  lib,
  ...
}:
{
  options.${namespace}.system.tuning.profile = lib.mkOption {
    type = lib.types.enum [
      "desktop"
      "laptop"
      "server"
    ];
    default = "desktop";
    description = "Workload the network and VM sysctls are tuned for.";
  };
}
