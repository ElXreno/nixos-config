usage() {
  echo "usage: textify FILE [--lines N] [--from N]" >&2
  exit 2
}

[[ $# -ge 1 ]] || usage
f=$1
shift
lines=400
from=1
while [[ $# -gt 0 ]]; do
  case $1 in
    --lines) lines=$2; shift 2 ;;
    --from) from=$2; shift 2 ;;
    *) usage ;;
  esac
done
[[ -f $f ]] || { echo "textify: no such file: $f" >&2; exit 1; }

mime=$(file -b --mime-type "$f")
out=$(mktemp)
trap 'rm -f "$out"' EXIT

case $mime in
  application/pdf)
    pdftotext -layout "$f" "$out"
    if [[ $(tr -d '[:space:]' <"$out" | wc -c) -lt 200 ]]; then
      echo "[no text layer, running OCR]" >&2
      ocrmypdf --quiet --force-ocr -l rus+eng --sidecar "$out" "$f" /dev/null
    fi
    ;;
  text/html | application/xhtml+xml)
    trafilatura --links <"$f" >"$out" 2>/dev/null || true
    [[ -s $out ]] || html2text "$f" >"$out"
    ;;
  image/*)
    tesseract "$f" - -l rus+eng >"$out" 2>/dev/null
    ;;
  application/zip | application/x-7z-compressed | application/x-rar | application/vnd.rar | \
    application/x-tar | application/gzip | application/x-xz | application/x-bzip2 | application/zstd | \
    application/x-iso9660-image | application/vnd.ms-cab-compressed)
    7z l "$f" >"$out" 2>&1 || lsar -l "$f" >"$out" 2>&1
    ;;
  application/msword | application/vnd.openxmlformats-officedocument.* | application/vnd.oasis.opendocument.* | \
    application/vnd.ms-excel | application/vnd.ms-powerpoint | application/rtf | text/rtf)
    d=$(mktemp -d)
    soffice --headless --convert-to txt:Text --outdir "$d" "$f" >/dev/null 2>&1
    cat "$d"/* >"$out" 2>/dev/null
    rm -rf "$d"
    ;;
  text/* | application/json | application/xml | application/javascript)
    cp "$f" "$out"
    ;;
  *)
    {
      echo "[binary: $(file -b "$f")]"
      exiftool -S "$f" 2>/dev/null | head -40
      echo "[binwalk]"
      binwalk "$f" 2>/dev/null | head -40
      echo "[strings]"
      strings -n 8 "$f"
    } >"$out"
    ;;
esac

total=$(wc -l <"$out")
tail -n +"$from" "$out" | head -n "$lines"
to=$((from + lines - 1))
if [[ $total -gt $to ]]; then
  echo "[lines $from-$to of $total; next: textify $f --from $((to + 1))]"
fi
