_:

final: prev: {
  mpv = prev.mpv.override {
    mpv = prev.mpv-unwrapped.override {
      vapoursynthSupport = true;
      vapoursynth = final.vapoursynth.withPlugins [
        final.vapoursynth-mvtools
        final.elxreno-nix.vs-rife-ncnn-vulkan
      ];
    };
    scripts = with final.mpvScripts; [
      mpris
      sponsorblock
      quality-menu
    ];
    youtubeSupport = true;
  };
}
