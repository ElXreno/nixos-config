_:

_final: prev: {
  # Samsung phones flap TDLS every ~10s and break WiFi performance.
  wpa_supplicant = prev.wpa_supplicant.overrideAttrs (oldAttrs: {
    extraConfig = oldAttrs.extraConfig + ''
      undefine CONFIG_TDLS
    '';
  });
}
