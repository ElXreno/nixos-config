export LC_ALL=C

json=$(cat)

readarray -t f < <(@jq@ -r '
  [(.workspace.current_dir // .cwd // ""),
   (.model.display_name // ""),
   (.effort.level // ""),
   (.context_window.used_percentage // ""),
   (.cost.total_cost_usd // ""),
   (.rate_limits.five_hour.used_percentage // ""),
   (.rate_limits.seven_day.used_percentage // ""),
   (.pr.number // ""),
   (.pr.url // ""),
   (.pr.review_state // "")]
  | map(tostring) | .[]' <<<"$json")

dir=${f[0]} model=${f[1]} effort=${f[2]} ctx=${f[3]} cost=${f[4]}
rl5=${f[5]} rl7=${f[6]} pr_num=${f[7]} pr_url=${f[8]} pr_state=${f[9]}

dim=$'\e[2m' reset=$'\e[0m' bold=$'\e[1m'
green=$'\e[32m' yellow=$'\e[33m' red=$'\e[31m'

seg=()

if [[ -n $dir ]]; then
  s=$(basename "$dir")
  branch=$(@git@ -C "$dir" --no-optional-locks branch --show-current 2>/dev/null)
  if [[ -z $branch ]]; then
    branch=$(@git@ -C "$dir" --no-optional-locks rev-parse --short HEAD 2>/dev/null)
  fi
  if [[ -n $branch ]]; then
    star=""
    [[ -n $(@git@ -C "$dir" --no-optional-locks status --porcelain 2>/dev/null | head -c1) ]] && star="*"
    s="$s ($branch$star)"
  fi
  seg+=("$bold$s$reset")
fi

if [[ -n $model ]]; then
  seg+=("$model${effort:+ $effort}")
fi

if [[ -n $ctx ]]; then
  p=${ctx%%.*}
  c=$green
  ((p >= 50)) && c=$yellow
  ((p >= 80)) && c=$red
  seg+=("ctx $c$p%$reset")
fi

if [[ -n $cost ]]; then
  printf -v costf '$%.2f' "$cost"
  seg+=("$costf")
fi

rl=""
if [[ -n $rl5 ]]; then
  printf -v v '%.0f' "$rl5"
  rl="5h $v%"
fi
if [[ -n $rl7 ]]; then
  printf -v v '%.0f' "$rl7"
  rl+="${rl:+ · }7d $v%"
fi
[[ -n $rl ]] && seg+=("$rl")

if [[ -n $pr_num ]]; then
  case $pr_state in
  approved) sc=$green ;;
  changes_requested) sc=$red ;;
  draft) sc=$dim ;;
  *) sc=$yellow ;;
  esac
  t="PR #$pr_num"
  [[ -n $pr_state ]] && t+=" $pr_state"
  if [[ -n $pr_url ]]; then
    seg+=("$sc"$'\e]8;;'"$pr_url"$'\e\\'"$t"$'\e]8;;\e\\'"$reset")
  else
    seg+=("$sc$t$reset")
  fi
fi

flag="${CLAUDE_CONFIG_DIR:-$HOME/.claude}/.caveman-active"
if [[ -f $flag && ! -L $flag ]]; then
  mode=$(head -c 64 "$flag" 2>/dev/null | tr -d '\n\r' | tr '[:upper:]' '[:lower:]' | tr -cd 'a-z0-9-')
  orange=$'\e[38;5;172m'
  case $mode in
  full) seg+=("${orange}[CAVEMAN]$reset") ;;
  lite | ultra | wenyan | wenyan-lite | wenyan-full | wenyan-ultra | commit | review | compress)
    seg+=("${orange}[CAVEMAN:${mode^^}]$reset")
    ;;
  esac
fi

out=""
for s in "${seg[@]}"; do
  out+="${out:+$dim | $reset}$s"
done
printf '%s' "$out"
