_:

_final: prev: {
  duperemove = prev.duperemove.overrideAttrs (prevAttrs: {
    patches = (prevAttrs.patches or [ ]) ++ [
      (prev.fetchpatch {
        url = "https://github.com/markfasheh/duperemove/commit/ad301cd577dbbcb03d41d870de946b9e2cd2552b.patch";
        hash = "sha256-+zmpZkXBo1JDAYObd4TZuuuDG/a73KF0Zqf/qXqy9mQ=";
      })
      (prev.fetchpatch {
        url = "https://github.com/markfasheh/duperemove/commit/72f75e63d22c807fe745ee22ca94473c16b0ec04.patch";
        hash = "sha256-T9+1YVIHpI8k8C7CUfJ1p7Jo1UZK0ivpa1Hp9VanfGA=";
      })
    ];
  });
}
