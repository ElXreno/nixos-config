{
  lib,
  fetchFromGitHub,
  runCommandLocal,
  nodejs,
  lessc,
  colors ? null,
  styles ? null,
  excludeStyles ? [ ],
  accent ? "blue",
  styleVars ? { },
}:
let
  src = fetchFromGitHub {
    owner = "catppuccin";
    repo = "userstyles";
    rev = "36e8398623e38f045d8845519d3268d9e45999b9";
    hash = "sha256-bQdSt5qW3R0jAcBiciX3s2/omugDSjlTM5a5mGsWVBE=";
  };

  slots = lib.genList (i: "base0${lib.toHexString i}") 16;

  scripts = lib.fileset.toSource {
    root = ./.;
    fileset = lib.fileset.unions [
      ./build.mjs
      ./palette.mjs
    ];
  };
in
assert lib.assertMsg (
  colors == null || lib.all (s: colors ? ${s}) slots
) "userstyles-base16: colors is missing some of base00..base0F";
assert lib.assertMsg (
  colors == null || styles != [ ]
) "userstyles-base16: styles must not be empty, null builds every style";

runCommandLocal "userContent.css"
  {
    nativeBuildInputs = [
      nodejs
      lessc
    ];
    passAsFile = [
      "palette"
      "vars"
    ];
    palette = builtins.toJSON (if colors == null then { } else lib.getAttrs slots colors);
    vars = builtins.toJSON {
      global = {
        lightFlavor = "mocha";
        darkFlavor = "mocha";
        accentColor = accent;
      };
      perStyle = styleVars;
    };
    styles = if styles == null then "" else styles;
    inherit excludeStyles;
  }
  (
    if colors == null then
      "touch $out"
    else
      ''
        if [ -z "$styles" ]; then
          for dir in ${src}/styles/*; do
            [ -d "$dir" ] || continue
            style=$(basename "$dir")
            case " $excludeStyles " in
              *" $style "*) ;;
              *) styles="$styles $style" ;;
            esac
          done
          [ -n "$styles" ] || { echo "every style is excluded"; exit 1; }
        fi

        node ${scripts}/build.mjs ${src} "$palettePath" "$varsPath" $out $styles

        lessc --no-color $out > /dev/null \
          || { echo "the generated stylesheet does not parse"; exit 1; }

        for style in $styles; do
          grep -q "^/\* $style \*/$" $out || { echo "$style is missing from the output"; exit 1; }
        done
        echo "generated $(wc -c < $out) bytes for: $styles"
      ''
  )
