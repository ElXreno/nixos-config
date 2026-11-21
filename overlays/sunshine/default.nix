_:

final: prev: {
  sunshine = prev.sunshine.overrideAttrs (
    _finalAttrs: prevAttrs: {
      patches = (prevAttrs.patches or [ ]) ++ [ ./unicode-via-wtype.patch ];

      postPatch = (prevAttrs.postPatch or "") + ''
        substituteInPlace src/platform/virtualhid_input.cpp \
          --replace-fail '@wtype@' '${prev.lib.getExe final.wtype}'
      '';
    }
  );
}
