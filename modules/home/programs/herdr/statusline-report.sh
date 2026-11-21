json=$(cat)
[[ -n ${HERDR_PANE_ID:-} ]] || exit 0

ctx=$(@jq@ -r '.context_window.used_percentage // empty' <<<"$json")

meta=()
[[ -n $ctx ]] && meta+=(--token "ctx=${ctx%%.*}%")

since=$(@herdr@ pane get "$HERDR_PANE_ID" 2>/dev/null | @jq@ -r '.result.pane.tokens.busy_since // empty')
if [[ -n $since ]]; then
  d=$(($(@date@ +%s) - since))
  ((d < 0)) && d=0
  if ((d >= 3600)); then
    printf -v busy '%dh%02dm' $((d / 3600)) $((d % 3600 / 60))
  elif ((d >= 60)); then
    printf -v busy '%dm%02ds' $((d / 60)) $((d % 60))
  else
    printf -v busy '%ds' "$d"
  fi
  meta+=(--token "busy=$busy")
fi

if ((${#meta[@]})); then
  @herdr@ pane report-metadata "$HERDR_PANE_ID" \
    --source user:claude-statusline \
    --agent claude \
    "${meta[@]}" \
    --ttl-ms 86400000 >/dev/null 2>&1 || true
fi
