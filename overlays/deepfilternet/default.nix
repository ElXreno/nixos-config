_:

_final: prev: {
  deepfilternet = prev.deepfilternet.overrideAttrs (prevAttrs: {
    patches = (prevAttrs.patches or [ ]) ++ [ ./ladspa-worker-wakeup.patch ];
  });
}
