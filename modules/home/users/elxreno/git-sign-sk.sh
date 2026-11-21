sign=0
key=
prev=
for arg in "$@"; do
  if [[ $prev == -Y && $arg == sign ]]; then sign=1; fi
  if [[ $prev == -f ]]; then key=$arg; fi
  prev=$arg
done

if ((sign == 0)) || [[ ! -r $key ]]; then
  exec ssh-keygen "$@"
fi

silent_key() {
  local private=$1 silent=$2
  if [[ -e $silent && ! $private -nt $silent ]]; then return 0; fi
  if install -d -m 700 "${silent%/*}" &&
    install -m 600 "$private" "$silent" &&
    ssh-keygen -q -p -O no-touch-required -P "" -N "" -f "$silent" >/dev/null 2>&1; then
    return 0
  fi
  rm -f "$silent"
  return 1
}

private=${key%.pub}
runtime=${XDG_RUNTIME_DIR:-}
if [[ -n $runtime && -r $private ]]; then
  silent=$runtime/git-sign-sk/${private##*/}
  if silent_key "$private" "$silent"; then
    args=()
    prev=
    for arg in "$@"; do
      if [[ $prev == -f ]]; then args+=("$silent"); else args+=("$arg"); fi
      prev=$arg
    done
    SSH_AUTH_SOCK='' ssh-keygen "${args[@]}" 2>/dev/null && exit 0
  fi
fi

ssh-keygen "$@" && exit 0
rc=$?

blob=$(awk '{print $2; exit}' "$key")
mapfile -t socks < <(
  find "$HOME/.ssh/agent" -maxdepth 1 -type s -name 's.*.sshd.*' -printf '%T@ %p\n' 2>/dev/null |
    sort -rn | cut -d' ' -f2-
)

for sock in "${socks[@]}"; do
  keys=$(SSH_AUTH_SOCK=$sock timeout 2 ssh-add -L 2>/dev/null) || continue
  if [[ $keys == *"$blob"* ]]; then
    SSH_AUTH_SOCK=$sock exec ssh-keygen "$@"
  fi
done

echo "security key did not sign: touch it when it blinks if it is plugged in here, or connect from the phone with agent forwarding" >&2
exit "$rc"
