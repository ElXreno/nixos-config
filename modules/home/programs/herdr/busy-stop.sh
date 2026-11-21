cat >/dev/null
[ -n "${HERDR_PANE_ID:-}" ] || exit 0

@herdr@ pane report-metadata "$HERDR_PANE_ID" \
  --source user:claude-busy \
  --agent claude \
  --clear-token busy \
  --clear-token busy_since >/dev/null 2>&1 || true
