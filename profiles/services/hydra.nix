{ config, pkgs, lib, ... }:

let
  sshConfig = pkgs.writeText "ssh-config" ''
    Host builder1-x86_64
      HostName 100.78.254.88
      IdentityFile ${config.sops.secrets."ssh/distributed-builds".path}
      Port 22

    Host builder2-x86_64
      HostName 100.69.244.92
      IdentityFile ${config.sops.secrets."ssh/distributed-builds".path}
      Port 22

    Host builder3-x86_64
      HostName 100.81.8.111
      IdentityFile ${config.sops.secrets."ssh/distributed-builds".path}
      Port 22

    Host *
      Compression yes
      ControlMaster auto
      ControlPath ~/.ssh/master-%r@%n:%p
      ControlPersist 10m
      ForwardAgent no
      IdentitiesOnly yes
      StrictHostKeyChecking accept-new
      User builder
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
        hostName = "builder1-x86_64";
        systems = [ "x86_64-linux" "i686-linux" ];
        maxJobs = 8;
        speedFactor = 8;
        supportedFeatures = [ "nixos-test" "benchmark" "big-parallel" "kvm" ];
        mandatoryFeatures = [ ];
      }
      {
        hostName = "builder2-x86_64";
        systems = [ "x86_64-linux" "i686-linux" ];
        maxJobs = 6;
        speedFactor = 4;
        supportedFeatures = [ "nixos-test" "benchmark" "big-parallel" "kvm" ];
        mandatoryFeatures = [ ];
      }
      {
        hostName = "builder3-x86_64";
        systems = [ "x86_64-linux" "i686-linux" ];
        maxJobs = 6;
        speedFactor = 4;
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
