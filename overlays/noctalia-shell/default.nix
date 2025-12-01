{ inputs, ... }:

_final: prev: {
  noctalia-shell =
    inputs.noctalia.packages.${prev.stdenv.hostPlatform.system}.default.overrideAttrs
      (oldAttrs: {
        patches = [
          ./0001-SystemStatService-Add-acpitz-for-temp-cpu-sensors-li.patch
        ];
      });
}
