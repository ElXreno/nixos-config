_:

_final: prev: {
  bitmagnet = prev.bitmagnet.overrideAttrs (
    _finalAttrs: prevAttrs: {
      version = "unstable-2025-08-01";

      src = prev.fetchFromGitHub {
        owner = "bitmagnet-io";
        repo = "bitmagnet";
        rev = "2b9e8eadd34c037830d1fa7470b5ef2746cd6388";
        hash = "sha256-FPHBu/SdfnXICqPxEfLUsWNMwZqQ5PR6ATGFaeHuGAU=";
      };

      vendorHash = "sha256-aWFh3vytRARFEnVxTtSkvBOXZP0ke9e602BVNQ6xoRY=";

      patches = (prevAttrs.patches or [ ]) ++ [
        (prev.fetchpatch {
          url = "https://github.com/bitmagnet-io/bitmagnet/pull/435/commits/8c8fdcde9a6b6f40a83870981aefee65f9521f31.patch";
          hash = "sha256-jFAsiMWjsOY0axkv7xSTrzVR66wri9fEGRhRz+5LwTs=";
        })
        (prev.fetchpatch {
          url = "https://github.com/bitmagnet-io/bitmagnet/pull/435/commits/61e92b7edc6549d0c12956a02828abb62438ca1f.patch";
          hash = "sha256-nErbPtdcnCyhDrNjpGJYb73YAsF3IrVwc39EfJd2EBE=";
        })
      ];
    }
  );
}
