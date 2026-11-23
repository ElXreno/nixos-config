{ inputs, ... }:
_final: prev:
let
  withArcLru =
    zfs:
    zfs.overrideAttrs (old: {
      patches = (old.patches or [ ]) ++ inputs.zfs-slab-mobility.patches.zfs.arclru;
    });
in
{
  zfs_2_4-arclru = withArcLru prev.zfs_2_4;

  kernelPackagesExtensions = prev.kernelPackagesExtensions ++ [
    (_self: kp: {
      zfs_2_4-arclru = withArcLru kp.zfs_2_4;
    })
  ];
}
