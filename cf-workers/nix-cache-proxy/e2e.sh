#!/usr/bin/env bash
set -euo pipefail

cd "$(dirname "$0")"
port=${PORT:-8787}
base=http://127.0.0.1:$port

wrangler dev --ip 127.0.0.1 --port "$port" --show-interactive-dev-session=false >/dev/null 2>&1 &
trap 'kill $!' EXIT
until curl --silent --fail --output /dev/null "$base/nix-cache-info"; do
  sleep 1
done

keys=$(curl --silent --fail "$base/" | sed -n 's/^trusted-public-keys = //p')
nix store verify --store "$base" --option trusted-public-keys "$keys" --sigs-needed 1 "$@"
