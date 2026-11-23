_:

final: prev:
let
  runtimeInputs = with final; [
    glib
    gpu-screen-recorder
    slurp
    sqlite
    udiskie
    xdg-user-dirs
    xdg-utils
  ];
in
{
  noctalia =
    (prev.noctalia.override { stdenv = final.ccacheTools.wrapStdenv prev.stdenv; }).overrideAttrs
      (prevAttrs: {
        patches = (prevAttrs.patches or [ ]) ++ [
          ./zfs-arc-lru.patch
        ];
        postFixup = prevAttrs.postFixup + ''
          wrapProgram $out/bin/noctalia \
            --suffix PATH : ${final.lib.makeBinPath runtimeInputs}
        '';
      });
}
