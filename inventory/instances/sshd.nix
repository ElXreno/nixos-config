{
  inventory.instances.sshd = {
    roles.server.tags = [ "all" ];
    roles.server.settings.authorizedKeys.root = (import ../_ssh-keys.nix).desktop;
  };
}
