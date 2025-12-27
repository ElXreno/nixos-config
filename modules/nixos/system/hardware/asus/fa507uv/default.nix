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
    concatStringsSep
    mkForce
    genAttrs
    ;
  cfg = config.${namespace}.system.hardware.asus.fa507uv;

  patchesSrc = pkgs.fetchFromGitHub {
    owner = "CachyOS";
    repo = "kernel-patches";
    rev = "bfcf34bd22aa1fa740c5d60a8f126919cfdacfdf";
    hash = "sha256-YdhrS8JBGnM4BvdkG0MbO8I4dJLmF+RyP7VCRCf7LVQ=";
  };

  extraPatches = [
    "${patchesSrc}/${majorMinor}/0001-asus.patch"
    ./kernel-patches/0001-platform-x86-asus-armoury-Add-tunings-for-FA507UV-bo.patch
    ./kernel-patches/iwlwifi-lar_disable.patch
  ];

  baseKernel = pkgs.linux_xanmod_latest.overrideAttrs (prevAttrs: {
    patches = (prevAttrs.patches or [ ]) ++ extraPatches;
  });
  majorMinor = lib.versions.majorMinor baseKernel.version;

  extraKernelModules = [
    # Some SATA/PATA stuff.
    "ahci"
    "sata_nv"
    "sata_via"
    "sata_sis"
    "sata_uli"
    "ata_piix"

    # Standard SCSI stuff.
    "sd_mod"
    "sr_mod"

    # SD cards and internal eMMC drives.
    "mmc_block"

    # For nbfc
    "acpi_ec"
  ];

  initrdMissingModules = [
    "pata_marvell"

    "ehci_hcd"
    "ehci_pci"
    "ohci_hcd"
    "ohci_pci"
    "uhci_hcd"

    "mmc_block"

    "hid_apple"
    "hid_cherry"
    "hid_corsair"
    "hid_lenovo"
    "hid_logitech_dj"
    "hid_logitech_hidpp"
    "hid_microsoft"
    "hid_roccat"

    "pcips2"
  ];

  combinedModprobedDb = pkgs.runCommand "combined-modprobed-db" { } ''
    echo -e "${concatStringsSep "\n" extraKernelModules}\n$(cat ${./modprobed.db})" | sort -u > $out
  '';

  pkgbuildCompact = [
    "-d GENERIC_CPU"
    "-e MZEN4"

    "-e ASUS_ARMOURY"
    "-e ASUS_LAPTOP"
    "-e ASUS_WIRELESS"

    "-m NTSYNC" # Maybe it useless as we already has `ntsync` at `modprobed.db`
  ];

  minimizedConfig = pkgs.stdenv.mkDerivation {
    inherit (baseKernel) src patches;
    name = "${baseKernel.name}-minimized-config";

    nativeBuildInputs = with baseKernel; nativeBuildInputs ++ buildInputs;

    buildPhase = ''
      cp "${baseKernel.configfile}" ".config"
      make LSMOD="${combinedModprobedDb}" localmodconfig

      make olddefconfig
      patchShebangs scripts/config
      scripts/config ${lib.concatStringsSep " " pkgbuildCompact}
      make olddefconfig
    '';

    installPhase = ''
      cp .config $out
    '';
  };

  finalKernel =
    (pkgs.linuxManualConfig {
      inherit (baseKernel) version src modDirVersion;

      configfile = minimizedConfig;
      allowImportFromDerivation = true;
    }).overrideAttrs
      (prevAttrs: {
        patches = (prevAttrs.patches or [ ]) ++ extraPatches;
        passthru = baseKernel.passthru;
      });
in
{
  options.${namespace}.system.hardware.asus.fa507uv = {
    enable = mkEnableOption "Whether or not to manage ASUS FA507UV stuff.";
  };

  config = mkIf cfg.enable {
    ${namespace}.system.boot.kernel.packages = pkgs.linuxPackagesFor finalKernel;
    boot.initrd.availableKernelModules =
      let
        mkForceDisable = modules: genAttrs modules (_: mkForce false);
      in
      mkForceDisable initrdMissingModules;

    boot.initrd.prepend = [
      # Fix D3cold power state loop (D0 -> D3cold -> D0), thanks ASUS
      # For reference: Remove Notify (\_SB.NPCF, 0xC0) in the `_OFF` method of \_SB.PCI0.GPP0 scope
      "${pkgs.runCommand "acpi-overrides" { buildInputs = with pkgs; [ cpio ]; } ''
        mkdir -p kernel/firmware/acpi
        cp ${./acpi/ssdt4.aml} kernel/firmware/acpi/ssdt4.aml
        find kernel | cpio -H newc -o > $out
      ''}"
    ];

    boot.extraModprobeConfig = ''
      options cfg80211 ieee80211_regdom=US
      options iwlwifi lar_disable=1
    '';
  };
}
