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
  cfg = config.${namespace}.programs.haven;
in
{
  options.${namespace}.programs.haven = {
    enable = mkEnableOption "provisioning the Haven agent endpoint token";

    user = mkOption {
      type = types.str;
      default = config.${namespace}.primaryUser;
      defaultText = lib.literalExpression "config.\${namespace}.primaryUser";
      description = "User allowed to read the pairing token.";
    };
  };

  config = mkIf cfg.enable {
    clan.core.vars.generators.haven-mcp = {
      files.token = {
        owner = cfg.user;
      };
      prompts.token = {
        description = ''
          Provide the pairing token minted by Haven's agent endpoint.
          Enable Settings -> Agent endpoint on the phone, then run the MCP `initialize`
          request once and take the token from `.result._meta."sh.haven/pairingToken"`.
        '';
        type = "hidden";
        persist = true;
      };
    };
  };
}
