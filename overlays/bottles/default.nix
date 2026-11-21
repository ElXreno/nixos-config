_:

_final: prev:
let
  fileLandlockFixed = prev.file.overrideAttrs (old: {
    # Work around too strict landlock hardening blocking the bzip2 helper exec
    # https://bugs.astron.com/view.php?id=785
    postPatch = (old.postPatch or "") + ''
      substituteInPlace src/landlock.c --replace-fail \
        "LANDLOCK_ACCESS_FS_READ_FILE | LANDLOCK_ACCESS_FS_READ_DIR" \
        "LANDLOCK_ACCESS_FS_READ_FILE | LANDLOCK_ACCESS_FS_READ_DIR | LANDLOCK_ACCESS_FS_EXECUTE"
    '';
  });
in
{
  bottles = prev.bottles.override {
    removeWarningPopup = true;
  };

  bottles-unwrapped = prev.bottles-unwrapped.override {
    python3Packages = prev.python3Packages // {
      patool = prev.python3Packages.patool.override { file = fileLandlockFixed; };
    };
  };
}
