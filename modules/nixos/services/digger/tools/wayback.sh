usage() {
  cat >&2 <<'EOF'
usage:
  wayback list URL-OR-PREFIX [--exact] [--ext EXT] [--mime TYPE] [--from YYYY] [--to YYYY] [--limit N] [--all-status]
  wayback get TIMESTAMP URL [-o FILE]
EOF
  exit 2
}

cdx() {
  local target=$1
  shift
  local match=prefix ext='' mime='' from='' to='' limit=200 status=1
  while [[ $# -gt 0 ]]; do
    case $1 in
      --exact) match=exact; shift ;;
      --ext) ext=$2; shift 2 ;;
      --mime) mime=$2; shift 2 ;;
      --from) from=$2; shift 2 ;;
      --to) to=$2; shift 2 ;;
      --limit) limit=$2; shift 2 ;;
      --all-status) status=0; shift ;;
      *) usage ;;
    esac
  done
  local args=(--get --silent --show-error --max-time 120 --retry 3 --retry-delay 5
    --data-urlencode "url=$target"
    --data-urlencode "matchType=$match"
    --data-urlencode "output=json"
    --data-urlencode "fl=timestamp,original,mimetype,statuscode,length"
    --data-urlencode "collapse=digest"
    --data-urlencode "limit=$limit")
  [[ $status == 1 ]] && args+=(--data-urlencode "filter=statuscode:200")
  [[ -n $ext ]] && args+=(--data-urlencode "filter=original:.*\\.${ext}([?#].*)?$")
  [[ -n $mime ]] && args+=(--data-urlencode "filter=mimetype:$mime")
  [[ -n $from ]] && args+=(--data-urlencode "from=$from")
  [[ -n $to ]] && args+=(--data-urlencode "to=$to")
  curl "${args[@]}" "https://web.archive.org/cdx/search/cdx" | jq -r '
    if length <= 1 then "no captures"
    else .[1:][] | "\(.[0])  \(.[3])  \(.[2])  \(.[4])  \(.[1])" end'
}

case ${1:-} in
  list)
    shift
    [[ $# -ge 1 ]] || usage
    cdx "$@"
    ;;
  get)
    shift
    [[ $# -ge 2 ]] || usage
    ts=$1 url=$2
    shift 2
    exec grab "https://web.archive.org/web/${ts}id_/${url}" "$@"
    ;;
  *) usage ;;
esac
