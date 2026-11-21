cat >/dev/null
[ -n "${HERDR_PANE_ID:-}" ] || exit 0

@herdr@ pane report-metadata "$HERDR_PANE_ID" \
  --source user:claude-busy \
  --agent claude \
  --token busy_since="$(@date@ +%s)" \
  --token busy=0s \
  --ttl-ms 86400000 >/dev/null 2>&1 || true
