{
  inputs,
  namespace,
  cudaSupport ? false,
  ...
}:
system:
let
  inherit (inputs.nixpkgs) lib;

  collectModules =
    dir:
    builtins.filter (p: lib.hasSuffix ".nix" p && !lib.hasPrefix "_" (baseNameOf p)) (
      lib.filesystem.listFilesRecursive dir
    );

  mkOverlays = map (f: import f { inherit inputs; }) (collectModules ./overlays);

  packageDirs = lib.mapAttrsToList (name: _: ./packages + "/${name}") (
    lib.filterAttrs (
      name: type: type == "directory" && builtins.pathExists (./packages + "/${name}/default.nix")
    ) (builtins.readDir ./packages)
  );

  mkPackagesOverlay =
    final: _prev:
    let
      callPackage = lib.callPackageWith (final // { inherit inputs namespace; });
      mkPackage = dir: lib.nameValuePair (baseNameOf dir) (callPackage dir { });
    in
    {
      ${namespace} = lib.listToAttrs (map mkPackage packageDirs);
    };

in
rec {
  pkgs = import inputs.nixpkgs {
    inherit system;

    overlays = [ mkPackagesOverlay ] ++ mkOverlays;

    config = {
      allowUnfree = true;
      inherit cudaSupport;
      nvidia.acceptLicense = true;
    };
  };

  packages = pkgs.${namespace};
}
