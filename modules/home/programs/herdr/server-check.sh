probe() {
  @timeout@ 2 @herdr@ workspace list >/dev/null 2>&1
}

case ${1-} in
absent)
  ! probe
  ;;
ready)
  for _ in {1..150}; do
    probe && exit 0
    @sleep@ 0.2
  done
  exit 1
  ;;
*)
  exit 2
  ;;
esac
