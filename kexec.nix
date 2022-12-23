{ config, pkgs, ... }: # https://gist.github.com/misuzu/89fb064a2cc09c6a75dc9833bb3995bf
{
  imports = [
    # this will work only under qemu, uncomment next line for full image
    # <nixpkgs/nixos/modules/installer/netboot/netboot-minimal.nix>
    <nixpkgs/nixos/modules/installer/netboot/netboot.nix>
    <nixpkgs/nixos/modules/profiles/qemu-guest.nix>
  ];

  # stripped down version of https://github.com/cleverca22/nix-tests/tree/master/kexec
  system.build = rec {
    image = pkgs.runCommand "image" { buildInputs = [ pkgs.nukeReferences ]; } ''
      mkdir $out
      cp ${config.system.build.kernel}/${config.system.boot.loader.kernelFile} $out/kernel
      cp ${config.system.build.netbootRamdisk}/initrd $out/initrd
      nuke-refs $out/kernel
    '';
    kexec_script = pkgs.writeTextFile {
      executable = true;
      name = "kexec-nixos";
      text = ''
        #!${pkgs.stdenv.shell}
        set -e
        ${pkgs.kexectools}/bin/kexec -l ${image}/kernel --initrd=${image}/initrd --append="init=${builtins.unsafeDiscardStringContext config.system.build.toplevel}/init ${toString config.boot.kernelParams}"
        sync
        echo "executing kernel, filesystems will be improperly umounted"
        ${pkgs.kexectools}/bin/kexec -e
      '';
    };
    kexec_tarball = pkgs.callPackage <nixpkgs/nixos/lib/make-system-tarball.nix> {
      storeContents = [
        {
          object = config.system.build.kexec_script;
          symlink = "/kexec_nixos";
        }
      ];
      contents = [ ];
      compressCommand = "cat";
      compressionExtension = "";
    };
    kexec_tarball_self_extract_script = pkgs.writeTextFile {
      executable = true;
      name = "kexec-nixos";
      text = ''
        #!/bin/sh
        set -eu
        ARCHIVE=`awk '/^__ARCHIVE_BELOW__/ { print NR + 1; exit 0; }' $0`
        tail -n+$ARCHIVE $0 | tar x -C /
        /kexec_nixos $@
        exit 1
        __ARCHIVE_BELOW__
      '';
    };
    kexec_bundle = pkgs.runCommand "kexec_bundle" { } ''
      cat \
        ${kexec_tarball_self_extract_script} \
        ${kexec_tarball}/tarball/nixos-system-${kexec_tarball.system}.tar \
        > $out
      chmod +x $out
    '';
  };

  boot.initrd.availableKernelModules = [ "ata_piix" "uhci_hcd" ];
  boot.kernelParams = [
    "panic=30" "boot.panic_on_fail" # reboot the machine upon fatal boot issues
    "console=ttyS0" # enable serial console
    "console=tty1"
  ];
  boot.kernel.sysctl."vm.overcommit_memory" = "1";

  environment.systemPackages = with pkgs; [ cryptsetup btrfs-progs ];
  environment.variables.GC_INITIAL_HEAP_SIZE = "1M";

  networking.hostName = "kexec";

  services.getty.autologinUser = "root";
  services.openssh = {
    enable = true;
    challengeResponseAuthentication = false;
    passwordAuthentication = false;
  };

  documentation.enable = false;
  documentation.nixos.enable = false;
  fonts.fontconfig.enable = false;
  programs.bash.enableCompletion = false;
  programs.command-not-found.enable = false;
  security.polkit.enable = false;
  security.rtkit.enable = pkgs.lib.mkForce false;
  services.udisks2.enable = false;
  i18n.supportedLocales = [ (config.i18n.defaultLocale + "/UTF-8") ];

  users.users.root.openssh.authorizedKeys.keys = [
    "ecdsa-sha2-nistp521 AAAAE2VjZHNhLXNoYTItbmlzdHA1MjEAAAAIbmlzdHA1MjEAAACFBAH/QtzrqDZ/isIpMslg5FJvT6BoyeqpmiaDjuzcHaIpTexaq/UK4pAdG7IYvs++6JfdfAToWeU7TnOqRj8eubfFXADNwHC3w7gHjx/w8Yq76gcRG+UU/JtUbphzs2EdWWIupaZV+nFiTSbdGlak4fnnqSLIDhRgNa3pBbvSyf2OdD02bA== elxreno@desktop.local"
    "ecdsa-sha2-nistp256 AAAAE2VjZHNhLXNoYTItbmlzdHAyNTYAAAAIbmlzdHAyNTYAAABBBN4tzbWa0rhRF7oBin51NcO57YIeJ5oZsg4z4Uez0QNcGqKLfTr/oUGcSDsJZdKThdn55qegvacxD/LW0z50sDs= elxreno@Fujitsu-AH531-Laptop"
  ];

  zramSwap = {
    enable = true;
    algorithm = "lzo-rle";
    memoryPercent = 100;
  };
}
