{ inputs, ... }:
{
  imports = [ inputs.sops-nix.nixosModules.sops ];

  fileSystems."/var".neededForBoot = true; # Ensure that /var will be mounted with the sops key

  sops = {
    defaultSopsFile = ../secrets/common.yaml;
    age.sshKeyPaths = [ "/etc/ssh/ssh_host_ed25519_key" ];
    age.keyFile = "/var/lib/sops-nix/key";
    age.generateKey = true;
  };
}
