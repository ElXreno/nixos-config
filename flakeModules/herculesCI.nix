{ inputs, withSystem, ... }:
{
  flake.herculesCI = _: {
    onEvent.build_finished.github-status = withSystem "x86_64-linux" (
      { pkgs, ... }:
      (inputs.nixbot.lib.effects { inherit pkgs; }).mkEffect {
        name = "github-status";
        inputs = [
          pkgs.gh
          pkgs.jq
        ];
        secretsMap.github = "github-status";
        effectScript = ''
          case "$NIXBOT_BUILD_STATUS" in
            succeeded) state=success ;;
            failed) state=failure ;;
            *) exit 0 ;;
          esac
          GH_TOKEN=$(readSecretString github .token)
          export GH_TOKEN
          gh api "repos/ElXreno/nixos-config/statuses/$(jq -r .build.rev "$NIXBOT_EVENT_JSON")" \
            -f state="$state" \
            -f context=nixbot \
            -f description="nixbot build" \
            -f target_url="$NIXBOT_BUILD_URL" \
            --silent
        '';
      }
    );
  };
}
