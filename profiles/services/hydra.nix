{ config, pkgs, inputs, lib, ... }:

let
  sshConfig = pkgs.writeText "ssh-config" ''
    Host *
      Compression yes
      ControlMaster auto
      ControlPath ~/.ssh/master-%r@%n:%p
      ControlPersist 10m
      ForwardAgent no
      IdentitiesOnly yes
      StrictHostKeyChecking accept-new
      User builder
      IdentityFile ${config.sops.secrets."ssh/distributed-builds".path}
  '';
in {
  sops.secrets."ssh/distributed-builds" = { owner = "hydra-queue-runner"; };
  sops.secrets."hydra/github_bearer" = {
    restartUnits = [ "hydra-server.service" ];
  };
  sops.templates."hydra-extra-config" = {
    group = "hydra";
    mode = "440";
    content = ''
      <github_authorization>
        ElXreno = Bearer ${config.sops.placeholder."hydra/github_bearer"}
      </github_authorization>
    '';
  };

  services.hydra = {
    enable = true;
    hydraURL = "https://${config.device}.angora-ide.ts.net";
    notificationSender = "hydra@localhost";
    useSubstitutes = true;
    extraConfig = ''
      evaluator_workers = 8
      evaluator_max_memory_size = 4096
      max_concurrent_evals = 1

      <git-input>
        timeout = 3600
      </git-input>

      <githubstatus>
        jobs = .*
        useShortContext = 1
      </githubstatus>

      Include "${config.sops.templates."hydra-extra-config".path}"
    '';
  };

  services.caddy = {
    enable = true;
    virtualHosts."${config.device}.angora-ide.ts.net" = {
      extraConfig = ''
        encode zstd gzip
        handle {
          reverse_proxy :${toString config.services.hydra.port}
        }
      '';
    };
  };

  systemd.services.hydra-evaluator =
    lib.mkIf (config.services.hydra.enable && config.device == "flamingo") {
      # https://github.com/NixOS/hydra/issues/1186
      environment.GC_DONT_GC = "1";
      serviceConfig.CPUSchedulingPolicy = "idle";
      serviceConfig.IOSchedulingClass = "idle";
    };

  system.activationScripts.setupHydraSshConfig = lib.stringAfter [ "var" ] ''
    mkdir -p ${config.users.users.hydra-queue-runner.home}/.ssh/
    chown -Rv hydra-queue-runner ${config.users.users.hydra-queue-runner.home}/.ssh
    ln -svf ${sshConfig} ${config.users.users.hydra-queue-runner.home}/.ssh/config
  '';

  nix = {
    distributedBuilds = true;

    buildMachines = [
      {
        hostName = "100.81.15.62";
        systems = [ "aarch64-linux" ];
        maxJobs = 8;
        speedFactor = 1;
        supportedFeatures = [ "nixos-test" "benchmark" "big-parallel" "kvm" ];
        mandatoryFeatures = [ ];
      }
      {
        hostName = "100.90.64.52";
        systems = [ "x86_64-linux" ];
        maxJobs = 2;
        speedFactor = 1;
        supportedFeatures = [ "nixos-test" "benchmark" "big-parallel" "kvm" ];
        mandatoryFeatures = [ ];
      }
    ];

    settings = {
      allowed-uris = [
        "https://github.com/"
        "https://git.sr.ht/"
        "github:"
        "git+https://github.com"
        "git+ssh://github.com"
      ];
    };
  };
}
