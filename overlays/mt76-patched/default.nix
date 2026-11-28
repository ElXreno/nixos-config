_: _final: prev: {
  kernelPackagesExtensions = prev.kernelPackagesExtensions ++ [
    (_self: kp: {
      mt76-patched = kp.callPackage (import ../_kernel-subtree-module.nix {
        pname = "mt76-patched";
        subdir = "drivers/net/wireless/mediatek/mt76";
        patches = [
          (prev.fetchpatch {
            name = "connac3-check-headroom-before-pushing-EHT-radiotap-TLVs.patch";
            url = "https://patchwork.kernel.org/project/linux-wireless/patch/20261001184529.502048-1-hoopyfrood42@gmail.com/raw/";
            hash = "sha256-v7xUf6umGPB5jjHgPel3EUL9CB+rLoVz1UlxmWY+35o=";
          })
          ./0001-wifi-mt76-mt7925-add-chip_addr-chip_mem-debugfs-for-.patch
          ./0002-wifi-mt76-mt7925-add-chip_config-debugfs-for-raw-fir.patch
          ./0003-wifi-mt76-mt7925-add-uni_cmd-debugfs-for-raw-UNI-com.patch
          ./0004-wifi-mt76-mt7925-arm-the-sniffer-when-a-monitor-vif-.patch
          ./0005-wifi-mt76-mt7925-tear-down-TDLS-links-the-peer-canno.patch
          ./0006-wifi-mt76-mt7925-add-debugfs-knobs-for-the-TDLS-weak.patch
        ];
        description = "mt76 kernel modules with upstream fixes and mt7925 debugfs diagnostics";
      }) { };
    })
  ];
}
