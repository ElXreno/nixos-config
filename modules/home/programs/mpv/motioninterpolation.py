import vapoursynth as vs
core = vs.core

clip = video_in

if clip.fps.numerator == 0:
    clip = core.std.AssumeFPS(clip, fpsnum=24000, fpsden=1001)

if clip.height > 720:
    clip = core.resize.Bicubic(clip, width=1280, height=720, format=vs.RGBS, matrix_in_s="709")
else:
    clip = core.resize.Bicubic(clip, format=vs.RGBS, matrix_in_s="709")

clip = core.rife.RIFE(
    clip,
    model=5,
    gpu_id=1,
    gpu_thread=2,
    sc=False
)

clip = core.resize.Bicubic(clip, format=vs.YUV420P8, matrix_s="709")

clip.set_output()
