#!/usr/bin/env bash
export LC_ALL=C

esc=$(printf '\033')

@jq@ -c --arg e "$esc" '
  def dim(s): $e + "[2m" + s + $e + "[0m";
  def bold(s): $e + "[1m" + s + $e + "[0m";
  def ktok: if . >= 100000 then ((. / 1000) | floor | tostring) + "k" else (((. / 100) | floor) / 10 | tostring) + "k" end;
  def pcol(p): if p >= 80 then $e + "[31m" elif p >= 50 then $e + "[33m" else $e + "[2m" end;
  (now * 1000) as $now
  | (.columns // 120) as $cols
  | .tasks[]?
  | (.label // .name // .description // "agent") as $title
  | (.description // "") as $desc0
  | (if $desc0 != "" and $desc0 != $title then $desc0 else "" end) as $desc
  | (.model // "" | sub("^claude-"; "") | sub("-[0-9].*$"; "")) as $m
  | (.tokenCount // 0) as $tok
  | (.contextWindowSize // 0) as $win
  | (if $win > 0 and $tok > 0 then ($tok * 100 / $win | floor) else null end) as $p
  | ((($now - (.startTime // $now)) / 1000) | floor) as $sec
  | (if $sec >= 3600
     then (($sec / 3600 | floor | tostring) + "h" + (($sec % 3600 / 60 | floor) | tostring | if length < 2 then "0" + . else . end) + "m")
     elif $sec >= 60
     then (($sec / 60 | floor | tostring) + "m" + (($sec % 60) | tostring | if length < 2 then "0" + . else . end) + "s")
     else (($sec | tostring) + "s")
     end) as $time
  | ([ (if $m != "" then [dim($m), $m] else empty end),
       (if $p != null and $p >= 10 then [pcol($p) + ($p | tostring) + "%" + $e + "[0m", ($p | tostring) + "%"] else empty end),
       [dim($time), $time],
       (if $tok > 0 then [dim("↓ " + ($tok | ktok) + " tokens"), "↓ " + ($tok | ktok) + " tokens"] else empty end)
     ]) as $parts
  | ($parts | map(.[0]) | join(dim(" · "))) as $right
  | ($parts | map(.[1]) | join(" · ") | length) as $rlen
  | ($cols - $rlen - ($title | length) - 4) as $dmax
  | (if $desc != "" and $dmax > 8
     then "  " + (if ($desc | length) > $dmax then ($desc | .[0:($dmax - 1)]) + "…" else $desc end)
     else "" end) as $descfit
  | ($cols - $rlen - 2) as $tmax
  | (if ($title | length) > $tmax then ($title | .[0:($tmax - 1)]) + "…" else $title end) as $tfit
  | ($cols - (($tfit + $descfit) | length) - $rlen) as $pad
  | { id: .id,
      content:
        (bold($tfit) + dim($descfit)
          + (" " * (if $pad > 1 then $pad else 1 end))
          + $right) }
'
