{
  config,
  namespace,
  lib,
  pkgs,
  ...
}:
base:
let
  inherit (config.${namespace}.system.hardware.gpu.nvidia) cudaCapability;
  name = "${base}-sm${lib.replaceStrings [ "." ] [ "" ] cudaCapability}";
in
pkgs.${namespace}.${name}
  or (throw "${namespace}: no package ${name} for CUDA capability ${cudaCapability}, add it to packages/${base}")
