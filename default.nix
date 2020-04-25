{ config, pkgs, lib, ...}:
let device = builtins.replaceStrings ["\n"] [""] (builtins.readFile /etc/hostname);
in
{
  imports = [
    "${./config}/${device}.nix"
    "${./hardware-config}/${device}.nix"
    <home-manager/nixos>
  ];
  
  system.stateVersion = "20.03";
}
