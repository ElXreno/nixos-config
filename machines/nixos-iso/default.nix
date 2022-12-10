{ inputs, ... }:

{
  imports = [
    "${inputs.nixpkgs}/nixos/modules/installer/cd-dvd/installation-cd-graphical-plasma5-new-kernel.nix"
    inputs.self.nixosRoles.iso
  ];

  isoImage.squashfsCompression = "zstd -Xcompression-level 4";
}
