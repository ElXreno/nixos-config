_:

_final: prev: {
  power-profiles-daemon-no-min-freq = prev.power-profiles-daemon.overrideAttrs (prevAttrs: {
    postPatch = (prevAttrs.postPatch or "") + ''
      substituteInPlace src/ppd-driver-amd-pstate.c \
        --replace-fail 'if (g_file_test (min_freq_path, G_FILE_TEST_EXISTS)) {' 'if (FALSE) {'
      substituteInPlace tests/integration_test.py \
        --replace-fail 'def test_amd_pstate_min_freq' 'def skip_amd_pstate_min_freq'
    '';
  });
}
