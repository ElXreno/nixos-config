{
  inputs,
  namespace,
  ...
}:
system:
let
  inherit (inputs.nixpkgs) lib;

  collectModules =
    dir: builtins.filter (lib.hasSuffix ".nix") (lib.filesystem.listFilesRecursive dir);

  mkOverlays = map (f: import f { inherit inputs; }) (collectModules ./overlays);

  packageDirs = map dirOf (collectModules ./packages);

  mkPackagesOverlay =
    final: _prev:
    let
      callPackage = lib.callPackageWith (final // { inherit inputs; });
      mkPackage = dir: lib.nameValuePair (baseNameOf dir) (callPackage dir { });
    in
    {
      ${namespace} = lib.listToAttrs (map mkPackage packageDirs);
    };

in
rec {
  pkgs = import inputs.nixpkgs {
    inherit system;

    overlays =
      with inputs;
      [
        firefox-addons.overlays.default
        mkPackagesOverlay
      ]
      ++ mkOverlays;

    config = {
      allowUnfree = true;
      nvidia.acceptLicense = true;
    };
  };

  packages = pkgs.${namespace};
}
