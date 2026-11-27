usage() {
  echo "usage: grab URL [-o NAME]" >&2
  exit 2
}

[[ $# -ge 1 ]] || usage
url=$1
shift
name=
while [[ $# -gt 0 ]]; do
  case $1 in
    -o) name=$2; shift 2 ;;
    *) usage ;;
  esac
done

dir=${DIGGER_FILES:-/work/files}
mkdir -p "$dir"
tmp=$(mktemp -p "$dir" .grab.XXXXXX)
trap 'rm -f "$tmp"' EXIT

meta=$(curl --location --silent --show-error --fail-with-body \
  --max-time 1800 --retry 3 --retry-delay 5 --compressed \
  --user-agent "${DIGGER_USER_AGENT:-Mozilla/5.0 (X11; Linux x86_64; rv:140.0) Gecko/20100101 Firefox/140.0}" \
  --remote-header-name --output "$tmp" \
  --write-out '%{http_code}\t%{content_type}\t%{url_effective}\t%header{content-disposition}' \
  "$url") || {
  echo "grab: failed: $meta" >&2
  head -c 2000 "$tmp" | strings >&2
  exit 1
}
IFS=$'\t' read -r code ctype final disposition <<<"$meta"

if [[ -z $name ]]; then
  name=$(sed -n 's/.*filename\*\?=\(UTF-8'"''"'\)\?"\?\([^";]*\)"\?.*/\2/ip' <<<"$disposition")
  [[ -n $name ]] || name=$(basename "${final%%[?#]*}")
  [[ -n $name && $name != / ]] || name=download
fi
name=${name//\//_}
target=$dir/$name
n=1
while [[ -e $target ]]; do
  target=$dir/${name%.*}.$n${name##*.}
  [[ $name == *.* ]] || target=$dir/$name.$n
  n=$((n + 1))
done
mv "$tmp" "$target"
trap - EXIT

sha=$(sha256sum "$target" | cut -d' ' -f1)
size=$(stat -c %s "$target")
kind=$(file -b "$target")
printf '%s\t%s\t%s\t%s\t%s\t%s\n' "$(date -u +%FT%TZ)" "$target" "$sha" "$size" "$url" "$final" >>"$dir/SOURCES.tsv"

echo "saved:   $target"
echo "http:    $code $ctype"
echo "final:   $final"
echo "size:    $size"
echo "sha256:  $sha"
echo "type:    $kind"
