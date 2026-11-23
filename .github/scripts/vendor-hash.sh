#!/usr/bin/env bash
set -euo pipefail

fake=sha256-AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA=

for mod in packages/*/go.mod; do
  dir=${mod%/go.mod}
  file=$dir/default.nix
  grep -q 'vendorHash = "sha256-' "$file" || continue

  sed -i "s|vendorHash = \"sha256-[^\"]*\";|vendorHash = \"$fake\";|" "$file"
  log=$(nix build --no-link ".#${dir#packages/}.goModules" 2>&1 || true)
  hash=$(sed -n 's/^ *got: *//p' <<<"$log")
  if [ -z "$hash" ]; then
    echo "$log" >&2
    exit 1
  fi
  sed -i "s|$fake|$hash|" "$file"
  echo "${dir#packages/}: $hash"
done
