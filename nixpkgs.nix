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
      mkPackages =
        dir:
        let
          file = import dir;
          shape = builtins.tryEval (file (lib.mapAttrs (name: _: throw name) (lib.functionArgs file)));
          result = callPackage dir { };
        in
        if shape.success && lib.isAttrs shape.value && !lib.isDerivation shape.value then
          lib.mapAttrs (name: _: result.${name}) shape.value
        else
          { ${baseNameOf dir} = result; };
    in
    {
      ${namespace} = lib.mergeAttrsList (map mkPackages packageDirs);
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
