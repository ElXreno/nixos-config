_:

_final: prev: {
  ffmpeg-full = prev.ffmpeg-full.override {
    withUnfree = true;
  };
}
