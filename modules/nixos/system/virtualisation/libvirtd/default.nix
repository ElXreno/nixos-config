{
  config,
  namespace,
  lib,
  pkgs,
  ...
}:
let
  inherit (lib)
    mkIf
    mkEnableOption
    ;
  cfg = config.${namespace}.system.virtualisation.libvirtd;
in
{
  options.${namespace}.system.virtualisation.libvirtd = {
    enable = mkEnableOption "Whether or not to manage libvirtd.";
  };

  config = mkIf cfg.enable {
    ${namespace}.system.impermanence.directories = [
      "/var/lib/libvirt"
    ];

    virtualisation = {
      spiceUSBRedirection.enable = true;
      libvirtd = {
        enable = true;
        qemu = {
          package = pkgs.qemu_kvm;
          swtpm.enable = true;
        };
      };
    };
  };
}
