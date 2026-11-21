summary=${1:?usage: update-packages SUMMARY.md}
updated=()
failed=()

packages=$(nix eval --impure --json --expr \
  "import $UPDATE_SELECT { flakePath = \"$PWD\"; system = \"$UPDATE_SYSTEM\"; }")

while IFS=$'\t' read -r name args; do
  before=$(nix eval --raw ".#$name.version")
  # shellcheck disable=SC2086
  if nix-update --flake $args "$name"; then
    after=$(nix eval --raw ".#$name.version")
    if [ "$before" != "$after" ]; then
      updated+=("| \`$name\` | \`$before\` → \`$after\` |")
    fi
  else
    git checkout -- "packages/$name"
    failed+=("\`$name\`")
  fi
done < <(jq -r 'to_entries[] | [.key, (.value | join(" "))] | @tsv' <<< "$packages")

: > "$summary"
if [ ${#updated[@]} -gt 0 ]; then
  {
    printf '| Package | Version |\n|---|---|\n'
    printf '%s\n' "${updated[@]}"
  } >> "$summary"
fi
if [ ${#failed[@]} -gt 0 ]; then
  printf '\nNot updated: %s\n' "${failed[*]}" >> "$summary"
fi
