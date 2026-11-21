cat >/dev/null
[ -n "${HERDR_PANE_ID:-}" ] || exit 0

since=$(@herdr@ pane get "$HERDR_PANE_ID" 2>/dev/null |
  @jq@ -r '.result.pane.tokens.busy_since // empty')
[ -n "$since" ] || exit 0

elapsed=$(($(@date@ +%s) - since))
[ "$elapsed" -ge 0 ] || elapsed=0

if [ "$elapsed" -ge 3600 ]; then
  busy=$(printf '%dh%02dm' $((elapsed / 3600)) $((elapsed % 3600 / 60)))
elif [ "$elapsed" -ge 60 ]; then
  busy=$(printf '%dm%02ds' $((elapsed / 60)) $((elapsed % 60)))
else
  busy="${elapsed}s"
fi

@herdr@ pane report-metadata "$HERDR_PANE_ID" \
  --source user:claude-busy \
  --agent claude \
  --token busy="$busy" \
  --ttl-ms 86400000 >/dev/null 2>&1 || true
