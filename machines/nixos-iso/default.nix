{
  config,
  inputs,
  pkgs,
  ...
}:
let
  asus_common = {
    services = {
      asusd = {
        enable = true;
        enableUserService = true;
      };
    };

    services.xserver.videoDrivers = [ "nvidia" ];
  };
in
{
  imports = [
    "${inputs.nixpkgs}/nixos/modules/installer/cd-dvd/installation-cd-graphical-calamares-plasma6.nix"
    inputs.self.nixosRoles.iso
  ];

  # isoImage.storeContents = [
  #   inputs.self.nixosConfigurations.INFINITY.config.system.build.toplevel
  #   inputs.self.nixosConfigurations.KURWA.config.system.build.toplevel
  # ];
  isoImage.squashfsCompression = "zstd -Xcompression-level 4";
  isoImage.edition = "graphical";

  specialisation = {
    Prestigio-Visconte-2.configuration = {
      isoImage.showConfiguration = true;
      isoImage.configurationName = "Prestigio Visconte 2 edition";

      environment.systemPackages = [
        pkgs.maliit-keyboard # Virtual Keyboard
      ];

      hardware.firmware = [ (pkgs.callPackage ./rtl8723b-firmware.nix { }) ];
    };

    asus-nvidia-open.configuration = {
      isoImage.showConfiguration = true;
      isoImage.configurationName = "ASUS with open-source NVidia ${config.hardware.nvidia.package.version} driver";

      hardware.nvidia.open = true;
    }
    // asus_common;
  };

  environment.systemPackages = with pkgs; [
    sbctl
  ];

  hardware.bluetooth = {
    enable = true;
    # For battery provider, bluezFull is just an alias for bluez
    package = pkgs.bluez-experimental;
    settings.General.Experimental = true;
  };
}
