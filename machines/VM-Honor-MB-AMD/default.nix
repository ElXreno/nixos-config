{ inputs, config, ... }:

{
  imports = [
    "${inputs.nixpkgs}/nixos/modules/profiles/qemu-guest.nix"
    "${inputs.nixpkgs}/nixos/modules/virtualisation/qemu-vm.nix"
    ../INFINITY
  ];

  config = {
    # Hack
    deviceSpecific.isLaptop = true;

    boot = {
      growPartition = true;
      kernelParams = [ "console=ttyS0" "boot.shell_on_fail" ];
    };

    virtualisation = {
      cores = 8;
      diskImage = "/home/elxreno/tmp/vm-${config.system.name}.qcow2";
      diskSize = 8192;
      memorySize = 4096;
      # useNixStoreImage = true;
    };
  };
}
