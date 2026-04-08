{ lib, ... }:
{
  _class = "clan.service";
  manifest.name = "pam-rssh";
  manifest.description = "pam_rssh on servers and ssh client match blocks to reach them";
  manifest.categories = [
    "System"
    "Network"
  ];
  manifest.readme = ''
    # pam-rssh

    Enables `pam_rssh` on machines in the `server` role so sudo can be
    authenticated via a forwarded SSH agent (pam_unix falls back when the
    agent has no matching key). Machines in the `client` role get a
    system-wide `/etc/ssh/ssh_config` Host block for every server in the
    instance, with `ForwardAgent yes`.
  '';

  roles.server = {
    description = "Accepts sudo elevation via forwarded SSH agent (pam_rssh).";
    perInstance = _: {
      nixosModule = _: {
        security.pam = {
          rssh.enable = true;
          services.sudo.rssh = true;
        };
      };
    };
  };

  roles.client = {
    description = "Writes /etc/ssh/ssh_config Host blocks for every server in this instance.";
    perInstance =
      { roles, ... }:
      {
        nixosModule =
          { config, ... }:
          let
            serverNames = lib.attrNames (roles.server.machines or { });
            inherit (config.clan.core.settings) domain;
          in
          {
            programs.ssh.extraConfig = lib.optionalString (serverNames != [ ]) ''
              Host ${lib.concatStringsSep " " (map lib.toLower serverNames)}
                Hostname %h.${domain}
                ForwardAgent yes
            '';
          };
      };
  };
}
