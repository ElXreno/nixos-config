{
  namespace,
  lib,
  ...
}:
let
  inherit (lib) mkOption types;
in
{
  options.${namespace}.services.kopia = {
    backup = mkOption {
      type = types.listOf (
        types.submodule { options = import ../../../../clanServices/kopia/source-options.nix lib; }
      );
      default = [ ];
      example = lib.literalExpression ''[ { path = "/var/lib/foo"; ignore = [ "/cache" ]; } ]'';
      description = ''
        Machine-wide paths to back up, declared by the service modules that own them.
        They land in the repository of the kopia instance's `systemUser`.
      '';
    };

    repositories = mkOption {
      type = types.listOf (
        types.submodule {
          options = {
            instance = mkOption { type = types.str; };
            user = mkOption { type = types.str; };
            prefix = mkOption { type = types.str; };
          };
        }
      );
      default = [ ];
      description = "Repositories this machine writes to, consumed by the bucket policy in terranix.";
    };
  };
}
