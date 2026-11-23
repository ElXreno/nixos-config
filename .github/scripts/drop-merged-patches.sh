#!/usr/bin/env bash
set -euo pipefail

rev=${1:-$(jq -r '.nodes[.nodes.root.inputs.nixpkgs].locked.rev' flake.lock)}

grep -o 'NixOS/nixpkgs/commit/[0-9a-f]\{40\}' flake.nix | cut -d/ -f4 | sort -u | while read -r sha; do
  status=$(gh api "repos/NixOS/nixpkgs/compare/$sha...$rev" --jq .status || true)
  case $status in
    ahead | identical) ;;
    *) continue ;;
  esac
  SHA=$sha perl -0pi -e 's/\n[ \t]*\(patcher\.fetchpatch \{[^}]*commit\/$ENV{SHA}\.patch[^}]*\}\)//' flake.nix
  echo "dropped $sha"
done

perl -0pi -e 's/\n[ \t]*# NixOS\/nixpkgs#\d+(?=\n[ \t]*(?:\n|\]|#))//g' flake.nix
