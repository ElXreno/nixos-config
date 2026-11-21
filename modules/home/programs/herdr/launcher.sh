if [[ -z ${HERDR_ENV-} && -z ${HERDR_SESSION-} ]] && [[ $# -eq 0 || $1 == remote-client-bridge ]]; then
  @systemctl@ --user start herdr.service >/dev/null 2>&1 || true
fi

exec @herdr@ "$@"
