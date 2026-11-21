payload=$(cat)
[ -n "${HERDR_PANE_ID:-}" ] || exit 0
[ -r "@tokenFile@" ] || exit 0

if [ "@onlyWhenDark@" = 1 ]; then
  connected=0
  lit=0

  for output in /sys/class/drm/card*-*/; do
    [ "$(cat "$output/status" 2>/dev/null)" = connected ] || continue
    connected=$((connected + 1))

    dpms=$(cat "$output/dpms" 2>/dev/null)
    enabled=$(cat "$output/enabled" 2>/dev/null)
    if [ "$dpms" != Off ] && [ "$enabled" != disabled ]; then
      lit=$((lit + 1))
    fi
  done

  if [ "$connected" -gt 0 ] && [ "$lit" -gt 0 ]; then
    exit 0
  fi
fi

message=$(printf '%s' "$payload" | @jq@ -r '.message // empty')

cwd=$(@herdr@ pane get "$HERDR_PANE_ID" 2>/dev/null |
  @jq@ -r '.result.pane.cwd // empty')
project=$(@basename@ "${cwd:-$PWD}")

request=$(@jq@ -nc --arg title "claude · $project" --arg body "${message:-Waiting for input}" '{
  jsonrpc: "2.0",
  id: 1,
  method: "tools/call",
  params: {
    name: "raise_notification",
    arguments: { title: $title, body: $body, priority: "high" },
  },
}')

@curl@ -sS -m 10 -X POST "@endpoint@" \
  -H "Authorization: Bearer $(cat "@tokenFile@")" \
  -H 'Content-Type: application/json' \
  -H 'Accept: application/json, text/event-stream' \
  -d "$request" >/dev/null 2>&1 || true
