PORT=""
MACHINE=""
VAR="zfs-passphrase/passphrase"

usage() {
    cat <<'EOF'
Usage: unlock-zfs [-p PORT] [-v VAR] -m MACHINE <host>

Unlock encrypted root on a remote server via initrd SSH.
Reads the passphrase from clan vars and pipes it to the
systemd password prompt automatically.

Options:
  -m MACHINE   clan machine name (required)
  -p PORT      initrd SSH port (default: boot.initrd.network.ssh.port of MACHINE)
  -v VAR       clan var path (default: zfs-passphrase/passphrase)
  -h, --help   show this help

Examples:
  nix run .#unlock-zfs -- -m BIMBA 1.2.3.4
  nix run .#unlock-zfs -- -m my-server -p 2222 -v luks/key my-server.example.com
EOF
    exit "${1:-0}"
}

while [[ $# -gt 0 ]]; do
    case "$1" in
        -m) MACHINE="${2:?-m requires a machine name}"; shift 2 ;;
        -p) PORT="${2:?-p requires a port number}"; shift 2 ;;
        -v) VAR="${2:?-v requires a var path}"; shift 2 ;;
        -h|--help) usage 0 ;;
        -*) echo "Unknown option: $1" >&2; usage 1 ;;
        *) HOST="$1"; shift ;;
    esac
done

: "${HOST:?No host specified. See --help.}"
: "${MACHINE:?No machine specified (-m). See --help.}"

if [[ -z $PORT ]]; then
    PORT=$(nix eval --raw ".#nixosConfigurations.$MACHINE.config.boot.initrd.network.ssh" --apply 's: if s.enable then toString s.port else throw "initrd SSH is not enabled on this machine; pass -p"')
fi
PASSPHRASE="$(clan vars get "$MACHINE" "$VAR")"

while ! ping -W 1 -c 1 "$HOST" >/dev/null; do
    sleep 1
done

while ! timeout --foreground 10 ssh -p "$PORT" "root@$HOST" true; do
    sleep 1
done

printf '%s\n' "$PASSPHRASE" | ssh -tt -p "$PORT" "root@$HOST" systemd-tty-ask-password-agent --query
