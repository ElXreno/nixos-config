{
  config,
  namespace,
  lib,
  pkgs,
  ...
}:

let
  inherit (lib)
    mkIf
    mkEnableOption
    mkOption
    types
    getExe
    optionalString
    escapeShellArg
    ;
  cfg = config.${namespace}.services.github-watch;

  github-watch = pkgs.writeShellApplication {
    name = "github-watch";
    runtimeInputs = [
      pkgs.gh
      pkgs.jq
      pkgs.coreutils
    ];
    text = ''
      ${optionalString (cfg.tokenFile != null) ''
        GH_TOKEN=$(< ${escapeShellArg cfg.tokenFile})
        export GH_TOKEN
      ''}
      owned=$(gh api --paginate "user/repos?affiliation=${cfg.affiliation}&per_page=100" --jq '.[].full_name' | sort)
      watched=$(gh api --paginate "user/subscriptions?per_page=100" --jq '.[].full_name' | sort)
      new=$(comm -23 <(printf '%s\n' "$owned") <(printf '%s\n' "$watched"))

      if [ -z "$new" ]; then
        echo "github-watch: nothing new"
        exit 0
      fi

      while read -r repo; do
        [ -n "$repo" ] || continue
        if gh api -X PUT "repos/$repo/subscription" -F subscribed=true -F ignored=false >/dev/null; then
          echo "github-watch: now watching $repo"
        else
          echo "github-watch: FAILED $repo" >&2
        fi
      done <<<"$new"
    '';
  };
in
{
  options.${namespace}.services.github-watch = {
    enable = mkEnableOption "Whether to auto-watch owned GitHub repos.";

    interval = mkOption {
      type = types.str;
      default = "*-*-* 00/6:00:00";
      description = "systemd OnCalendar expression for the watch sweep.";
    };

    affiliation = mkOption {
      type = types.str;
      default = "owner";
      description = "GitHub affiliation filter for repos to watch.";
    };

    tokenFile = mkOption {
      type = types.nullOr types.str;
      default = null;
      description = "Path to a file holding a GitHub token, exported as GH_TOKEN. Null uses gh's keyring login.";
    };
  };

  config = mkIf cfg.enable {
    systemd.user.services.github-watch = {
      Unit.Description = "Watch all owned GitHub repos";
      Service = {
        Type = "oneshot";
        ExecStart = getExe github-watch;
      };
    };

    systemd.user.timers.github-watch = {
      Unit.Description = "Periodic sweep to watch owned GitHub repos";
      Timer = {
        OnCalendar = cfg.interval;
        Persistent = true;
      };
      Install.WantedBy = [ "timers.target" ];
    };
  };
}
