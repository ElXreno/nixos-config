_:

{
  programs.ssh.extraConfig = ''
    Host builder-x86_64
      # ControlMaster auto
      # ControlPath ~/.ssh/master-%r@%n:%p
      # ControlPersist 6h

      Compression yes

      IdentitiesOnly yes
      User elxreno
      HostName 109.171.24.112
      Port 23
      IdentityFile /home/elxreno/.ssh/distributed-builds
      StrictHostKeyChecking accept-new
  '';

  nix = {
    buildMachines = [
      # {
      #   hostName = "builder-x86_64";
      #   systems = [ "x86_64-linux", "i686-linux" ];
      #   maxJobs = 6;
      #   speedFactor = 8;
      #   supportedFeatures = [ "nixos-test" "benchmark" "big-parallel" "kvm" ];
      #   mandatoryFeatures = [ ];
      # }
    ];

    # distributedBuilds = true;
  };
}
