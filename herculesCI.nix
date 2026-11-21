{ unpatchedInputs }:
let
  system = "x86_64-linux";
  pkgs = unpatchedInputs.nixpkgs.legacyPackages.${system};
  api = "http://127.0.0.1:${toString (import ./modules/nixos/services/nixbot/_port.nix)}/api";
in
_: {
  onEvent.build_finished.github-status =
    (unpatchedInputs.nixbot.lib.effects { inherit pkgs; }).mkEffect
      {
        name = "github-status";
        inputs = [
          pkgs.curl
          pkgs.gh
          pkgs.jq
        ];
        secretsMap.github = "github-status";
        effectScript = ''
          GH_TOKEN=$(readSecretString github .token)
          export GH_TOKEN
          rev=$(jq -r .build.rev "$NIXBOT_EVENT_JSON")

          report() {
            gh api "repos/ElXreno/nixos-config/statuses/$rev" \
              -f state="$1" -f context="$2" -f description="$3" -f target_url="$NIXBOT_BUILD_URL" \
              --silent
          }

          curl -fsS "${api}/''${NIXBOT_BUILD_URL#*://*/}" \
            | jq -r '.attributes[] | [.attr, .status] | @tsv' \
            | while IFS=$'\t' read -r attr status; do
              case "$status" in
                succeeded | skipped_local) state=success ;;
                failed | failed_eval | dependency_failed | cached_failure) state=failure ;;
                *) continue ;;
              esac
              report "$state" "nixbot/''${attr#checks.${system}.}" "$status"
            done

          case "$NIXBOT_BUILD_STATUS" in
            succeeded) report success nixbot "nixbot build" ;;
            failed) report failure nixbot "nixbot build" ;;
          esac
        '';
      };
}
