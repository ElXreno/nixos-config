{ inputs, ... }:

_final: prev: {
  inherit (inputs.hyprland.packages.${prev.stdenv.hostPlatform.system}) xdg-desktop-portal-hyprland;
}
