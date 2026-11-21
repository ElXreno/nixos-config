_:

_final: prev: {
  obs-studio-plugins = prev.obs-studio-plugins // {
    obs-advanced-masks = prev.obs-studio-plugins.obs-advanced-masks.overrideAttrs (prevAttrs: {
      patches = (prevAttrs.patches or [ ]) ++ [
        ./const-strrchr.patch
      ];
    });
  };
}
