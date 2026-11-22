{
  config,
  namespace,
  lib,
  pkgs,
  ...
}:
let
  inherit (lib)
    mkEnableOption
    mkForce
    mkIf
    mkOption
    types
    ;
  cfg = config.${namespace}.services.nixbot;
  niks3 = config.${namespace}.services.niks3;
  ports = config.${namespace}.ports;
  vars = config.clan.core.vars.generators;
in
{
  options.${namespace}.services.nixbot = {
    enable = mkEnableOption "the nixbot CI service, reachable inside the tailnet";
    repositories = mkOption {
      type = types.attrsOf (
        types.submodule {
          options = {
            url = mkOption {
              type = types.str;
              description = "Git URL polled for new commits.";
            };
            branch = mkOption {
              type = types.str;
              default = "main";
              description = "Branch whose head is built.";
            };
          };
        }
      );
      default = { };
      example = {
        "owner/repo".url = "https://github.com/owner/repo";
      };
      description = "Repositories nixbot polls and builds, keyed by project name.";
    };
    buildConcurrency = mkOption {
      type = types.ints.positive;
      default = 2;
      description = "Number of flake outputs built at the same time.";
    };
    maxJobs = mkOption {
      type = types.ints.positive;
      default = 3;
      description = "Number of derivations the nix daemon of this host builds at the same time.";
    };
    cores = mkOption {
      type = types.ints.positive;
      default = 8;
      description = "Number of threads one derivation may use on this host.";
    };
  };

  config = mkIf cfg.enable {
    ${namespace} = {
      ports.nixbot = 8010;
      services.postgresql = {
        enable = true;
        databases.nixbot = { };
      };
      system.impermanence.directories = [
        {
          directory = "/var/lib/nixbot";
          user = "nixbot";
          group = "nixbot";
          mode = "0700";
        }
      ];
      tailscale.serve."svc:nixbot" = {
        comment = "nixbot CI";
        endpoints."tcp:443" = "http://localhost:${toString ports.nixbot}";
      };
    };

    clan.core.vars.generators.nixbot-ssh = {
      files = {
        key = { };
        "key.pub".secret = false;
      };
      runtimeInputs = [ pkgs.openssh ];
      script = ''
        ssh-keygen -t ed25519 -N "" -C "nixbot@${config.clan.core.settings.machine.name}" -f "$out/key"
      '';
    };

    clan.core.vars.generators.nixbot-effects = {
      prompts.github-status-token = {
        description = "GitHub fine-grained token for the CI repositories with only 'Commit statuses: read and write'.";
        type = "hidden";
      };
      files."secrets.json" = { };
      runtimeInputs = [ pkgs.jq ];
      script = ''
        jq -n --rawfile token "$prompts/github-status-token" \
          '{ "github-status": { data: { token: ($token | rtrimstr("\n")) } } }' > "$out/secrets.json"
      '';
    };

    nix.settings = {
      max-jobs = cfg.maxJobs;
      inherit (cfg) cores;
    };

    services.nixbot = {
      enable = true;
      effects.perRepoSecretFiles = lib.mapAttrs' (
        name: _: lib.nameValuePair "pull_based:${name}" vars.nixbot-effects.files."secrets.json".path
      ) cfg.repositories;
      domain = "nixbot.${config.clan.core.settings.domain}";
      port = ports.nixbot;
      useHTTPS = true;
      nginx.enable = false;
      allowUnauthenticatedControl = true;
      buildSystems = [ config.nixpkgs.hostPlatform.system ];
      inherit (cfg) buildConcurrency;
      buildMaxSilentTime = 3600;
      database = {
        createLocally = false;
        url = "postgresql://nixbot@/nixbot?host=/run/postgresql";
      };
      pullBased = {
        sshPrivateKeyFile = vars.nixbot-ssh.files.key.path;
        sshKnownHostsFile = pkgs.writeText "nixbot-known-hosts" ''
          github.com ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAAIOMqqnkVzrm0SdG6UOoqKLsabgH5C9okWi0dh2l9GKJl
        '';
        repositories = lib.mapAttrs (_: repo: {
          inherit (repo) url;
          defaultBranch = repo.branch;
        }) cfg.repositories;
      };
      niks3 = {
        enable = true;
        serverUrl = niks3.url;
        authTokenFile = vars.niks3-api-token.files.token.path;
        inherit (config.services.niks3) package;
      };
    };

    systemd = {
      sockets.nixbot.socketConfig.ListenStream = mkForce "127.0.0.1:${toString ports.nixbot}";
      services.nixbot = {
        after = [ "postgresql.target" ];
        requires = [ "postgresql.target" ];
      };
    };
  };
}
