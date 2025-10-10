{
  lib,
  ...
}:

{
  imports = [ (lib.snowfall.fs.get-file "systems/x86_64-linux/INFINITY/default.nix") ];

  virtualisation = {
    cores = 4;
    memorySize = 4096;
    diskImage = null;
    resolution = {
      x = 1366;
      y = 768;
    };

    qemu.options = [
      "-device virtio-vga-gl"
      "-vga none"
      "-display gtk,gl=on,show-cursor=on"
    ];
  };
}
