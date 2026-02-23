{
  config,
  namespace,
  lib,
  ...
}:
let
  inherit (lib) mkOption types;

  cfg = config.${namespace}.system.virtualisation.clan-vm;
in
{
  options.${namespace}.system.virtualisation.clan-vm = {
    memorySize = mkOption {
      type = types.ints.positive;
      default = 6144;
      description = "The memory size in megabytes for the clan VM.";
    };

    cores = mkOption {
      type = types.ints.positive;
      default = 4;
      description = "Number of CPU cores for the clan VM.";
    };
  };

  config = {
    clan.virtualisation = {
      inherit (cfg) memorySize cores;
    };
  };
}
