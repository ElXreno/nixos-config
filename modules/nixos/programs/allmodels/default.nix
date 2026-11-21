{
  config,
  namespace,
  lib,
  ...
}:

let
  inherit (lib)
    mkIf
    mkEnableOption
    mkOption
    types
    ;
  cfg = config.${namespace}.programs.allmodels;
in
{
  options.${namespace}.programs.allmodels = {
    enable = mkEnableOption "provisioning the AllModels API key";
    user = mkOption {
      type = types.str;
      default = config.${namespace}.primaryUser;
      defaultText = lib.literalExpression "config.\${namespace}.primaryUser";
      description = "User allowed to read the API key.";
    };
  };

  config = mkIf cfg.enable {
    clan.core.vars.generators.allmodels = {
      files.api-key = {
        owner = cfg.user;
      };
      prompts.api-key = {
        description = "AllModels API key from https://allmodels.io";
        type = "hidden";
        persist = true;
      };
    };
  };
}
