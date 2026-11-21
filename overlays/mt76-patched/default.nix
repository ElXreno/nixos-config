_: _final: prev:
let
  wirelessNext =
    rev: hash:
    prev.fetchpatch {
      name = "${rev}.patch";
      url = "https://git.kernel.org/pub/scm/linux/kernel/git/wireless/wireless-next.git/patch/?id=${rev}";
      inherit hash;
    };
in
{
  kernelPackagesExtensions = prev.kernelPackagesExtensions ++ [
    (_self: kp: {
      mt76-patched = kp.callPackage (import ../_kernel-subtree-module.nix {
        pname = "mt76-patched";
        subdir = "drivers/net/wireless/mediatek/mt76";
        patches = [
          (wirelessNext "a105a9fe9a07a56d1e0b5a6978a49cde2ac7e7be" "sha256-7R65MLG4aAvfxPlCzrQdaWE7qqSmnjlJeLvV6ivu0hQ=")
          (wirelessNext "3208623121309a42f878364aff3c770f597d6a70" "sha256-5tLJKAY5seCiGVXKOotjiwTb/CpS5eGp4fBh85eXBQE=")
          (wirelessNext "d12a54be2604ecd23a1e9ac863583e34034aadde" "sha256-0nhLmepbZpeJesCgo2dnGV/C9rCvH5YV9cQZvY3OXWc=")
          (wirelessNext "8d2f5990cd429a2fe5fff05f7acf84d6799d4324" "sha256-BCHCRBU48VOfE2vkYJTPc5Q+GhNyvHxNDsCTo02wACM=")
          (wirelessNext "6f591a4bb5d9a6a4a28f3f2fd77f394ddbcc4e05" "sha256-YosZ3AdXTD1vtq+L2yrKzYVKZGMAVnOoKOgfpSLZc6g=")
          (wirelessNext "ac22c8c723c62013dd45b74e32249fc877c86fe0" "sha256-KO0kT9TL+Pko8bh8XiHIwMK2lUwA9hYZ4A0g/xa0rp0=")
          (wirelessNext "d6173b8b36e5cefc7cce34c31a0cc63fb3bd77f4" "sha256-BuFrUFjo7HGUyPJa9X7nPqbpeoGHjEP4BV4mVS0B1MA=")
          (wirelessNext "158c1145fe7ddd82914011a00de7cc3262616b16" "sha256-nBcpZxLNUeBAj9r8jky20OumgFX7tXoDKvvMjSjWILY=")
          (wirelessNext "5cebbd7a87df834efe002c8e54211e6adc99703a" "sha256-9QDxHtP3pnkntXQpgnGCyt2SDzGChtgG9vT1Mu/OO7E=")
          (wirelessNext "31b53a9c4ada2ce49201ed0da0105819acfd86bf" "sha256-VaUSSWirYeTgawTDN6Tyiipk9iyWESx9WNXG/r9itv8=")
          (wirelessNext "e1534fef1ae4df6e16401117394c26dbef84f361" "sha256-0Bmty+U7cfeHfhP+ii7rXfR46mBe2rgCWhgWfhY9DRs=")
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
