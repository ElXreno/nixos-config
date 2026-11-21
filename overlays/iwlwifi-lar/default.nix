_: _final: prev: {
  kernelPackagesExtensions = prev.kernelPackagesExtensions ++ [
    (_self: kp: {
      iwlwifi-lar = kp.callPackage (import ../_kernel-subtree-module.nix {
        pname = "iwlwifi-lar";
        subdir = "drivers/net/wireless/intel/iwlwifi";
        patches = [ ./lar_disable.patch ];
        postPatch = ''
          sed -i 's|$(srctree)/||' {d,m}vm/Makefile
        '';
        description = "iwlwifi kernel module rebuilt with the lar_disable modparam";
      }) { };
    })
  ];
}
