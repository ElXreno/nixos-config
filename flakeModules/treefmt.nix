{ unpatchedInputs, ... }:
{
  imports = [ unpatchedInputs.treefmt-nix.flakeModule ];

  perSystem =
    { lib, ... }:
    {
      treefmt = {
        projectRootFile = "flake.nix";
        programs = {
          nixfmt.enable = true;
          deadnix.enable = true;
          statix.enable = true;
          prettier = {
            enable = true;
            includes = [ "*.js" ];
          };
        };

        settings.formatter.deadnix.options = lib.mkForce [ ];
      };
    };
}
