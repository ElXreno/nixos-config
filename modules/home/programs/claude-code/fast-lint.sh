#!/usr/bin/env bash
set -uo pipefail

file=$(@jq@ -r '.tool_input.file_path // empty')
[ -n "$file" ] && [ -f "$file" ] || exit 0

name=$(basename "$file")
dir=$(dirname "$file")

has_config() {
  local d=$dir
  while [ "$d" != "/" ] && [ -n "$d" ]; do
    for c in "$@"; do
      [ -e "$d/$c" ] && return 0
    done
    [ -e "$d/.git" ] && return 1
    d=$(dirname "$d")
  done
  return 1
}

case "$name" in
  *.py)
    output=$(cd "$dir" && @ruff@ check --output-format concise --no-fix "$file" 2>&1)
    ;;
  Dockerfile | Dockerfile.* | *.dockerfile | *.Dockerfile)
    output=$(cd "$dir" && @hadolint@ --no-color "$file" 2>&1)
    ;;
  *.yaml | *.yml)
    has_config .yamllint .yamllint.yaml .yamllint.yml || exit 0
    output=$(cd "$dir" && @yamllint@ -f parsable "$file" 2>&1)
    ;;
  *.go)
    root=$dir
    while [ "$root" != "/" ] && [ ! -f "$root/go.mod" ]; do
      root=$(dirname "$root")
    done
    [ -f "$root/go.mod" ] || exit 0
    rel=${dir#"$root"}
    rel=${rel#/}
    output=$(cd "$root" && @timeout@ 10 @golangci@ run --show-stats=false "./${rel:-.}/" 2>&1)
    ;;
  *)
    exit 0
    ;;
esac
rc=$?
[ "$rc" -eq 0 ] && exit 0

printf '%s\n' "$output" >&2
exit 2
