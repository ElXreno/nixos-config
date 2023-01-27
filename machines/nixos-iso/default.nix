{ inputs, pkgs, ... }:

let rtl8723b-firmware = pkgs.callPackage ./rtl8723b-firmware.nix { };
in
{
  imports = [
    "${inputs.nixpkgs}/nixos/modules/installer/cd-dvd/installation-cd-graphical-plasma5-new-kernel.nix"
    inputs.self.nixosRoles.iso
  ];

  isoImage.squashfsCompression = "zstd -Xcompression-level 4";

  environment.systemPackages = with pkgs; [
    maliit-keyboard # Virtual keyboard
  ];

  hardware.firmware = [ rtl8723b-firmware ];

  hardware.bluetooth = {
    enable = true;
    # For battery provider, bluezFull is just an alias for bluez
    package = pkgs.bluez5-experimental;
    settings.General.Experimental = true;
  };
}
