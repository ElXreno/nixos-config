_:

_final: prev:
prev.lib.optionalAttrs (prev.stdenv.hostPlatform ? gcc.arch) {
  pythonPackagesExtensions = prev.pythonPackagesExtensions ++ [
    (_: python-super: {
      picosvg = python-super.picosvg.overridePythonAttrs (_: {
        disabledTestPaths = [ "tests/svg_test.py" ];
      });
    })
  ];

  xen = prev.xen.overrideAttrs (old: {
    patches = (old.patches or [ ]) ++ [
      (prev.fetchpatch {
        name = "xen-boot-64byte-align.patch";
        url = "https://gitlab.com/xen-project/patchew/xen/-/commit/3e1734ca29689424ed5372871a4c1b1bb978b84c.patch";
        hash = "sha256-fZ/U2iveeWLAJH9XDWkUQema6+ILd4HxnkNNy3rOVhc=";
      })
    ];
  });

  ncnn = prev.ncnn.overrideAttrs (old: {
    postPatch = (old.postPatch or "") + ''
      sed -i '1s|^|#if __AVX512F__\nstatic inline __m512 abs512_ps(__m512 x) { return _mm512_andnot_ps(_mm512_set1_ps(-0.f), x); }\n#endif\n|' \
        src/layer/x86/gemm_int8.h
    '';
  });

  libtpms = prev.libtpms.overrideAttrs (old: {
    env = (old.env or { }) // {
      NIX_CFLAGS_COMPILE = (old.env.NIX_CFLAGS_COMPILE or "") + " -Wno-error=stringop-overflow";
    };
  });

  nodejs_24 = prev.nodejs_24.overrideAttrs (_: {
    doCheck = false;
  });

  assimp = prev.assimp.overrideAttrs (_: {
    doCheck = false;
  });

  frei0r = prev.frei0r.overrideAttrs (old: {
    postPatch = (old.postPatch or "") + ''
      echo 'target_compile_options(tint0r PRIVATE -mno-sse4.1)' \
        >> src/filter/tint0r/CMakeLists.txt
    '';
  });

  gsl = prev.gsl.overrideAttrs (_: {
    doCheck = false;
  });

  simde = prev.simde.overrideAttrs (old: {
    postPatch = (old.postPatch or "") + ''
      sed -i "/subdir('test')/d" meson.build
    '';
  });

  lib2geom = prev.lib2geom.overrideAttrs (_: {
    doCheck = false;
  });
}
