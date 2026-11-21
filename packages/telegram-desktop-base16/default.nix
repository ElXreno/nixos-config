{
  lib,
  runCommand,
  nodejs,
  imagemagick,
  zopfli,
  telegram-desktop,
  colors ? null,
  chromaPull ? 0.6,
}:
let
  slots = lib.genList (i: "base0${lib.toHexString i}") 16;

  scripts = lib.fileset.toSource {
    root = ./.;
    fileset = lib.fileset.unions [
      ./inject.mjs
      ./theme.mjs
    ];
  };
in
assert lib.assertMsg (
  colors == null || lib.all (s: colors ? ${s}) slots
) "telegram-desktop-base16: colors is missing some of base00..base0F";

if colors == null then
  telegram-desktop
else
  runCommand telegram-desktop.name
    {
      nativeBuildInputs = [
        nodejs
        imagemagick
        zopfli
      ];
      passAsFile = [ "palette" ];
      palette = builtins.toJSON (lib.getAttrs slots colors);
      passthru = telegram-desktop.passthru or { };
      meta = telegram-desktop.meta or { };
    }
    ''
      cp -a ${telegram-desktop} $out
      chmod -R u+w $out
      node ${scripts}/inject.mjs $out ${telegram-desktop} "$palettePath" ${toString chromaPull}
    ''
