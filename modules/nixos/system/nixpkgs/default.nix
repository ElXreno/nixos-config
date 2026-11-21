{
  config,
  inputs,
  namespace,
  lib,
  ...
}:
let
  inherit (lib) mkIf mkEnableOption;
  cfg = config.${namespace}.nixpkgs;
in
{
  options.${namespace}.nixpkgs = {
    cudaSupport = mkEnableOption "the CUDA-enabled nixpkgs instance";
  };

  config = mkIf cfg.cudaSupport {
    nixpkgs.pkgs = inputs.self.legacyPackages.${config.nixpkgs.hostPlatform.system}.cuda;
  };
}
