{ config, pkgs, lib, ... }:

let
  sshConfig = pkgs.writeText "ssh-config" ''
    Host builder1-x86_64
      HostName eu.nixbuild.net
      IdentityFile ${config.sops.secrets."ssh/nixbuild".path}
      Port 22

    Host *
      Compression yes
      ControlMaster auto
      ControlPath ~/.ssh/master-%r@%n:%p
      ControlPersist 10m
      ForwardAgent no
      IdentitiesOnly yes
      StrictHostKeyChecking accept-new
      User elxreno
  '';
in
{
  sops.secrets."ssh/distributed-builds" = {
    owner = "hydra-queue-runner";
  };

  sops.secrets."ssh/nixbuild" = {
    owner = "hydra-queue-runner";
  };

  services.hydra = {
    enable = true;
    hydraURL = "http://localhost:3000";
    notificationSender = "hydra@localhost";
    useSubstitutes = true;
    extraConfig = ''
      <git-input>
        timeout = 3600
      </git-input>
    '';
  };

  system.activationScripts.setupHydraSshConfig = lib.stringAfter [ "var" ] ''
    mkdir -p ${config.users.users.hydra-queue-runner.home}/.ssh/
    chown -Rv hydra-queue-runner ${config.users.users.hydra-queue-runner.home}/.ssh
    ln -svf ${sshConfig} ${config.users.users.hydra-queue-runner.home}/.ssh/config
  '';

  nix = {
    buildMachines = [
      {
        hostName = "localhost";
        systems = [ "x86_64-linux" "i686-linux" ];
        maxJobs = 2;
        speedFactor = 2;
        supportedFeatures = [ "nixos-test" "benchmark" "big-parallel" "kvm" ];
        mandatoryFeatures = [ ];
      }
      {
        hostName = "builder1-x86_64";
        systems = [ "x86_64-linux" "i686-linux" ];
        maxJobs = 32;
        speedFactor = 8;
        supportedFeatures = [ "nixos-test" "benchmark" "big-parallel" "kvm" ];
        mandatoryFeatures = [ ];
      }
    ];

    settings = {
      allowed-uris = [
        "https://github.com/"
        "https://git.sr.ht/"
      ];
    };
  };
}
