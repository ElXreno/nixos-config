usage() {
  echo "usage: websearch [--lang ru|en|all] [--page N] [--n N] [--engines a,b] QUERY..." >&2
  exit 2
}

lang=all
page=1
n=20
engines=
while [[ $# -gt 0 ]]; do
  case $1 in
    --lang) lang=$2; shift 2 ;;
    --page) page=$2; shift 2 ;;
    --n) n=$2; shift 2 ;;
    --engines) engines=$2; shift 2 ;;
    -h|--help) usage ;;
    --) shift; break ;;
    *) break ;;
  esac
done
[[ $# -gt 0 ]] || usage
query="$*"

args=(--get --silent --show-error --max-time 30
  --data-urlencode "q=$query"
  --data-urlencode "format=json"
  --data-urlencode "pageno=$page"
  --data-urlencode "language=$lang")
[[ -n $engines ]] && args+=(--data-urlencode "engines=$engines")

curl "${args[@]}" "${SEARXNG_URL:-http://127.0.0.1:8888}/search" | jq -r --argjson n "$n" '
  (.results[:$n] | to_entries[] |
    "\(.key + 1). \(.value.title)\n   \(.value.url)\n   [\(.value.engines | join(","))] \(.value.content // "" | gsub("\\s+"; " ") | .[:300])\n"),
  (if (.unresponsive_engines | length) > 0
   then "unresponsive: \(.unresponsive_engines | map(join(": ")) | join("; "))" else empty end),
  (if (.results | length) == 0 then "no results" else empty end)'
