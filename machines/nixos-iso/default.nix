{
  inputs,
  pkgs,
  ...
}:

let
  rtl8723b-firmware = pkgs.callPackage ./rtl8723b-firmware.nix { };
in
{
  imports = [
    "${inputs.nixpkgs}/nixos/modules/installer/cd-dvd/installation-cd-graphical-calamares-plasma6.nix"
    inputs.self.nixosRoles.iso
  ];

  isoImage.storeContents = [ inputs.self.nixosConfigurations.INFINITY.config.system.build.toplevel ];
  isoImage.squashfsCompression = "zstd -Xcompression-level 4";

  environment.systemPackages = with pkgs; [
    maliit-keyboard # Virtual keyboard
    sbctl
  ];

  hardware.firmware = [ rtl8723b-firmware ];

  hardware.bluetooth = {
    enable = true;
    # For battery provider, bluezFull is just an alias for bluez
    package = pkgs.bluez5-experimental;
    settings.General.Experimental = true;
  };
}
