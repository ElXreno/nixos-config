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

  attic = inputs.attic.packages.${pkgs.system}.attic;

  upload-cache = pkgs.writeScript "upload-cache-to-attic" ''
    #!${pkgs.stdenv.shell}
    set -exu
    hydra_outputs=`cat $HYDRA_JSON | ${pkgs.jq}/bin/jq -r '.outputs[].path'`
    outputs=`${pkgs.nix}/bin/nix-store -qR --include-outputs $(${pkgs.nix}/bin/nix-store -qd $hydra_outputs) | ${pkgs.gnugrep}/bin/grep -v '\.drv$'`

    ${attic}/bin/attic push elxreno --ignore-upstream-cache-filter $outputs
  '';
in {
  sops.secrets."ssh/distributed-builds" = { owner = "hydra-queue-runner"; };

  services.hydra = {
    enable = true;
    hydraURL = "http://localhost:3000";
    notificationSender = "hydra@localhost";
    useSubstitutes = true;
    extraConfig = ''
      evaluator_workers = 8
      evaluator_max_memory_size = 4096
      max_concurrent_evals = 1

      <runcommand>
        job = *:*:*
        command = ${upload-cache}
      </runcommand>

      <git-input>
        timeout = 3600
      </git-input>
    '';
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
