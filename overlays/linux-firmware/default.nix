{ ... }:

_final: prev: {
  linux-firmware = prev.linux-firmware.overrideAttrs (prevAttrs: {
    src = prev.fetchurl {
      # https://www.kernel.org/pub/linux/kernel/firmware/
      # https://gitlab.com/kernel-firmware/linux-firmware
      url = "https://gitlab.com/kernel-firmware/linux-firmware/-/archive/881c549a82203abd9a88870ba27f3e8ce754b2c4/linux-firmware.tar.gz";
      # > nix store prefetch-file --hash-type sha256 https://gitlab.com/kernel-firmware/linux-firmware/-/archive/881c549a82203abd9a88870ba27f3e8ce754b2c4/linux-firmware.tar.gz
      hash = "sha256-R1LoLf0X9eLkuWjUkjJT7dboL3NF5fXnUia0oQucuyI=";
    };
  });
}
