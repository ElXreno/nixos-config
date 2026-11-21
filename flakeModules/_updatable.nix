{ flakePath, system }:
let
  flake = builtins.getFlake flakePath;
  packages = flake.packages.${system};
  prefix = "${flake.outPath}/packages/";
  own =
    name:
    let
      position = builtins.unsafeGetAttrPos "updateScript" packages.${name};
    in
    packages.${name} ? updateScript
    && position != null
    && builtins.substring 0 (builtins.stringLength prefix) position.file == prefix;
in
builtins.listToAttrs (
  map (name: {
    inherit name;
    value = builtins.tail packages.${name}.updateScript;
  }) (builtins.filter own (builtins.attrNames packages))
)
