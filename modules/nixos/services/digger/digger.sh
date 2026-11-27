die() {
  echo "digger: $*" >&2
  exit 1
}

usage() {
  cat >&2 <<EOF
usage:
  digger [-b] [-m MODEL] PROMPT...            dig in a new run and attach to its Claude Code
  digger attach [RUN]                         attach to a running run (detach: C-b d)
  digger run [-m MODEL] [-t MIN] -p PROMPT [-p PROMPT...]
                                              for agents: dig headless, wait for every run,
                                              print each report and the downloaded files
  digger resume RUN [-b] [-m MODEL] [PROMPT...] continue a finished run
  digger log [RUN]                            follow or print the log of a run
  digger list                                 runs, newest first
  digger stop RUN                             power off the VM of a run
  digger selftest                             boot a VM and check network isolation and tools

knowledge base of the diggers (every run digs with a copy, its additions wait for review):
  digger kb search QUERY...                   search it
  digger kb pending                           finished runs with unreviewed additions
  digger kb show RUN                          the additions of a run, in full
  digger kb accept [-o OUTCOME] [-n NOTE] RUN [KEY...]
                                              take the additions (all or KEYs) and record the dig
  digger kb reject [-o OUTCOME] [-n NOTE] RUN record the dig, drop the additions
  digger kb mem ARGS...                       any mem command on it: get, list, history, put, edit, retire

-b runs headless and prints the log instead of opening Claude Code.
OUTCOME of a dig: found, partial, not-found (default unverified).
RUN is a run name, a path, or "last" (the default).
MODEL is a peer or model id, default $defaultPeer:
$(printf '%s' "$models" | cut -f1,2 | sed 's/^/  /')
EOF
  exit 2
}

key=${XDG_STATE_HOME:-$HOME/.local/state}/digger/id_ed25519
kbDir=${XDG_DATA_HOME:-$HOME/.local/share}/digger
kbDb=$kbDir/kb.db

kbmem() {
  MEM_DB="" MEM_CORE=$kbDir/core.md XDG_STATE_HOME=$kbDir/state MEM_EMBED_URL="" MEM_RERANK_URL="" MEM_PROTECTED_DIRS="" \
    "$mem" --db "$kbDb" "$@"
}

runmem() {
  local run=$1
  shift
  MEM_DB="" MEM_CORE=$run/.kb/core.md XDG_STATE_HOME=$kbDir/state MEM_EMBED_URL="" MEM_RERANK_URL="" MEM_PROTECTED_DIRS="" \
    "$mem" --db "$run/.kb/mem.db" "$@"
}

kb_init() {
  if [[ ! -f $kbDb ]]; then
    mkdir -p "$kbDir"
    kbmem expire >/dev/null
  fi
}

kb_snapshot() {
  local run=$1
  [[ -f $run/.kb/mem.db ]] && return 0
  kb_init
  mkdir -p "$run/.kb"
  sqlite3 -cmd '.timeout 10000' "$kbDb" "VACUUM INTO '$run/.kb/mem.db'"
  date -u +%Y-%m-%dT%H:%M:%SZ >"$run/.kb/base"
}

kb_additions() {
  local run=$1
  [[ -f $run/.kb/mem.db && -f $run/.kb/base ]] || return 0
  sqlite3 -cmd '.timeout 10000' "$run/.kb/mem.db" 'PRAGMA wal_checkpoint(TRUNCATE)' >/dev/null
  kb_init
  kbmem merge --from "$run/.kb/mem.db" --since "$(cat "$run/.kb/base")" --dry-run | grep -v '^same' || true
}

model_line() {
  local peer model ctx search port
  while IFS=$'\t' read -r peer model ctx search port; do
    if [[ $peer == "$1" ]]; then
      printf '%s\t%s\t%s\t%s\t%s\n' "$peer" "$model" "$ctx" "$search" "$port"
      return 0
    fi
  done <<<"$models"
  while IFS=$'\t' read -r peer model ctx search port; do
    if [[ $model == "$1" ]]; then
      printf '%s\t%s\t%s\t%s\t%s\n' "$peer" "$model" "$ctx" "$search" "$port"
      return 0
    fi
  done <<<"$models"
  return 1
}

write_env() {
  local run=$1 want=$2 mode=$3 line peer model ctx search port
  line=$(model_line "$want") || die "no model $want"
  IFS=$'\t' read -r peer model ctx search port <<<"$line"
  printf 'PEER=%q\nMODEL=%q\nCTX=%q\nTOOL_SEARCH=%q\nBASE_PORT=%q\nMODE=%q\n' "$peer" "$model" "$ctx" "$search" "$port" "$mode" >"$run/run.env"
}

env_value() {
  sed -n "s/^$2=//p" "$1/run.env"
}

running() {
  local state
  state=$(systemctl is-active "microvm@digger-$1.service" || true)
  [[ $state == active || $state == activating || $state == deactivating ]]
}

slot_of() {
  local n
  for ((n = 0; n < slots; n++)); do
    if [[ $(cat "/run/digger/$n/target" 2>/dev/null) == "$1" ]] && running "$n"; then
      echo "$n"
      return 0
    fi
  done
  return 1
}

free_slot() {
  local n
  for ((n = 0; n < slots; n++)); do
    if ! running "$n"; then
      echo "$n"
      return 0
    fi
  done
  return 1
}

runs() {
  local d
  for d in "$runRoot"/*/; do
    [[ -d $d ]] && printf '%s\n' "${d%/}"
  done
}

resolve() {
  local arg=$1
  if [[ $arg == last ]]; then
    arg=$(runs | tail -n 1)
    [[ -n $arg ]] || die "no runs in $runRoot"
  fi
  if [[ -d $arg ]]; then
    realpath -- "$arg"
  elif [[ -d $runRoot/$arg ]]; then
    printf '%s\n' "$runRoot/$arg"
  else
    die "no run $arg"
  fi
}

summary() {
  if [[ -s $1/report.md ]]; then
    echo "report: $1/report.md"
  else
    echo "no report in $1"
  fi
}

show() {
  jq --unbuffered -R -r -f "$format"
}

transcript() {
  local f newest=''
  for f in "$1"/.claude/projects/*/*.jsonl; do
    [[ -f $f && ( -z $newest || $f -nt $newest ) ]] && newest=$f
  done
  printf '%s' "$newest"
}

log_file() {
  if [[ -s $1/agent.jsonl ]]; then
    printf '%s' "$1/agent.jsonl"
  else
    transcript "$1"
  fi
}

follow() {
  local run=$1 slot=$2 pid file
  pid=$(systemctl show -p MainPID --value "microvm@digger-$slot.service")
  file=$(log_file "$run")
  if [[ -z $file ]]; then
    file=$run/agent.jsonl
    touch "$file"
  fi
  if [[ $pid == 0 ]]; then
    show <"$file"
  else
    trap 'echo; echo "digger: stopped following, the run goes on: digger log $(basename "$run")" >&2; exit 0' INT
    tail -n +1 -F --pid="$pid" "$file" 2>/dev/null | show
    trap - INT
  fi
  summary "$run"
}

ensure_key() {
  if [[ ! -f $key ]]; then
    mkdir -p "$(dirname "$key")"
    ssh-keygen -q -t ed25519 -N '' -C digger -f "$key"
  fi
}

ssh_vm() {
  local slot=$1
  shift
  ssh -F /dev/null -i "$key" \
    -o IdentitiesOnly=yes -o IdentityAgent=none -o ForwardAgent=no -o ForwardX11=no -o ClearAllForwardings=yes \
    -o StrictHostKeyChecking=no -o UserKnownHostsFile=/dev/null -o LogLevel=ERROR -o ProxyUseFdpass=yes \
    -o ProxyCommand="$sshProxy vsock-mux/$stateDir/digger-$slot/notify.vsock %p" \
    "$@"
}

attach() {
  local run=$1 slot=$2 _
  for _ in $(seq 60); do
    ssh_vm "$slot" "digger@digger-$slot" true 2>/dev/null && break
    running "$slot" || break
    sleep 1
  done
  # shellcheck disable=SC2016
  ssh_vm "$slot" -t "digger@digger-$slot" \
    'for _ in $(seq 150); do for s in /tmp/digger.tmux /tmp/tmux-$(id -u)/dig; do tmux -S "$s" has-session -t dig 2>/dev/null && exec tmux -S "$s" attach -t dig; done; sleep 0.2; done; echo "digger: no Claude Code session in the VM" >&2; exit 1' || true
  sleep 1
  if running "$slot"; then
    echo "digger: detached, the run goes on: digger attach $(basename "$run")" >&2
  else
    summary "$run"
  fi
}

start() {
  local run=$1 slot
  slot=$(free_slot) || die "all $slots slots are busy"
  ensure_key
  mkdir -p "$run/.digger"
  chmod 0700 "$run/.digger"
  install -m 0600 "$key.pub" "$run/.digger/authorized_keys"
  printf '%s\n' "$run" >"/run/digger/$slot/target"
  echo "digger: $(basename "$run") on slot $slot" >&2
  systemctl start "microvm@digger-$slot.service" || die "the VM on slot $slot did not start: journalctl -u microvm@digger-$slot"
  if [[ $(env_value "$run" MODE) == batch ]]; then
    follow "$run" "$slot"
  else
    attach "$run" "$slot"
  fi
}

slugify() {
  local s
  s=$(printf '%s' "$1" | tr '[:upper:]' '[:lower:]' | sed -E 's/[^a-z0-9]+/-/g; s/^-+//' | cut -c1-40)
  s=${s%%-}
  printf '%s' "${s:-run}"
}

cmd_new() {
  local model=$defaultPeer mode=tui prompt run
  while [[ ${1:-} == -* ]]; do
    case $1 in
      -m)
        [[ $# -ge 2 ]] || usage
        model=$2
        shift 2
        ;;
      -b)
        mode="batch"
        shift
        ;;
      *) usage ;;
    esac
  done
  prompt="$*"
  if [[ -z $prompt && ! -t 0 ]]; then
    prompt=$(cat)
  fi
  [[ -n $prompt ]] || usage
  model_line "$model" >/dev/null || die "no model $model"
  run=$runRoot/$(date +%Y-%m-%d-%H%M%S)-$(slugify "$prompt")
  mkdir -p "$run"
  printf '%s\n' "$prompt" >"$run/prompt.md"
  write_env "$run" "$model" "$mode"
  kb_snapshot "$run"
  start "$run"
}

cmd_resume() {
  local run model mode=tui prompt
  [[ $# -ge 1 ]] || usage
  run=$(resolve "$1")
  shift
  model=$(env_value "$run" PEER)
  while [[ ${1:-} == -* ]]; do
    case $1 in
      -m)
        [[ $# -ge 2 ]] || usage
        model=$2
        shift 2
        ;;
      -b)
        mode="batch"
        shift
        ;;
      *) usage ;;
    esac
  done
  if slot_of "$run" >/dev/null; then
    die "$(basename "$run") is running: digger attach $(basename "$run")"
  fi
  prompt="$*"
  if [[ -z $prompt && $mode == batch ]]; then
    prompt="Continue the task: re-read /work/notes.md and work through the open leads."
  fi
  printf '%s\n' "$prompt" >"$run/followup.md"
  write_env "$run" "$model" "$mode"
  kb_snapshot "$run"
  rm -f "$run/.kb/reviewed"
  start "$run"
}

cmd_attach() {
  local run slot
  run=$(resolve "${1:-last}")
  slot=$(slot_of "$run") || die "$(basename "$run") is not running: digger resume $(basename "$run")"
  [[ $(env_value "$run" MODE) != batch ]] || die "$(basename "$run") runs headless: digger log $(basename "$run")"
  ensure_key
  attach "$run" "$slot"
}

cmd_log() {
  local run slot file
  run=$(resolve "${1:-last}")
  if slot=$(slot_of "$run"); then
    follow "$run" "$slot"
  else
    file=$(log_file "$run")
    [[ -n $file ]] || die "$(basename "$run") has no log"
    show <"$file"
    summary "$run"
  fi
}

cmd_list() {
  local run slot state
  runs | tac | while read -r run; do
    if slot=$(slot_of "$run"); then
      state="running on slot $slot"
    elif [[ -e $run/selftest ]]; then
      if [[ -s $run/selftest.txt && ! -e $run/ssh-failed ]] && ! grep -q '^FAIL' "$run/selftest.txt"; then
        state="selftest passed"
      else
        state="selftest failed"
      fi
    elif [[ -s $run/report.md ]]; then
      state="done"
    else
      state=unfinished
    fi
    printf '%s\t%s\n' "$(basename "$run")" "$state"
  done
}

host_reachable() {
  timeout 5 bash -c "</dev/tcp/$1/$2" 2>/dev/null
}

selftest_targets() {
  local label ip port
  while read -r label ip port; do
    [[ -n $ip && $ip != null ]] || continue
    if host_reachable "$ip" "$port"; then
      printf '%s %s %s\n' "$label" "$ip" "$port"
    else
      echo "digger: control $label $ip:$port is not reachable from the host either, skipped" >&2
    fi
  done <<EOF
router $(ip -j route show default | jq -r '.[0].gateway // empty') 80
lan-dns $(ip -j route show default | jq -r '.[0].gateway // empty') 53
tailnet-dns 100.100.100.100 53
tailnet-self $(ip -j addr show tailscale0 2>/dev/null | jq -r '.[0].addr_info[]? | select(.family == "inet") | .local') 22
EOF
}

cmd_selftest() {
  local run slot
  run=$runRoot/$(date +%Y-%m-%d-%H%M%S)-selftest
  mkdir -p "$run"
  selftest_targets >"$run/selftest"
  write_env "$run" "$defaultPeer" batch
  slot=$(free_slot) || die "all $slots slots are busy"
  ensure_key
  mkdir -p "$run/.digger"
  chmod 0700 "$run/.digger"
  install -m 0600 "$key.pub" "$run/.digger/authorized_keys"
  printf '%s\n' "$run" >"/run/digger/$slot/target"
  echo "digger: selftest on slot $slot" >&2
  systemctl start "microvm@digger-$slot.service"
  local _ ssh=FAIL
  for _ in $(seq 20); do
    if ssh_vm "$slot" "digger@digger-$slot" true 2>/dev/null; then
      ssh=ok
      break
    fi
    sleep 1
  done
  printf '%-5s ssh over vsock\n' "$ssh"
  [[ $ssh == ok ]] || touch "$run/ssh-failed"
  while running "$slot"; do
    sleep 1
  done
  cat "$run/selftest.txt" 2>/dev/null || die "no selftest result: journalctl -u microvm@digger-$slot"
  if [[ -e $run/ssh-failed ]] || grep -q '^FAIL' "$run/selftest.txt"; then
    return 1
  fi
}

new_run() {
  local prompt=$1 model=$2 mode=$3 run
  run=$runRoot/$(date +%Y-%m-%d-%H%M%S)-$(slugify "$prompt")
  while [[ -e $run ]]; do
    sleep 1
    run=$runRoot/$(date +%Y-%m-%d-%H%M%S)-$(slugify "$prompt")
  done
  mkdir -p "$run/.digger"
  chmod 0700 "$run/.digger"
  ensure_key
  install -m 0600 "$key.pub" "$run/.digger/authorized_keys"
  printf '%s\n' "$prompt" >"$run/prompt.md"
  write_env "$run" "$model" "$mode"
  kb_snapshot "$run"
  printf '%s\n' "$run"
}

result() {
  local run=$1 status=$2 additions
  echo "== $(basename "$run"): $status"
  echo "run directory: $run"
  if [[ -s $run/report.md ]]; then
    echo "-- report.md"
    cat "$run/report.md"
  else
    echo "-- no report.md"
  fi
  if [[ -s $run/files/SOURCES.tsv ]]; then
    echo "-- files (time, path, sha256, bytes, url, final url)"
    cat "$run/files/SOURCES.tsv"
  fi
  additions=$(kb_additions "$run")
  echo "-- knowledge base additions (review: digger kb show $(basename "$run"), then digger kb accept or reject)"
  printf '%s\n' "${additions:-none}"
  echo
}

cmd_run() {
  local model=$defaultPeer minutes=60 prompts=() runs=() slot_run=() started=() deadline=() status=() i n next=0 done_count=0
  while [[ $# -gt 0 ]]; do
    case $1 in
      -m)
        [[ $# -ge 2 ]] || usage
        model=$2
        shift 2
        ;;
      -t)
        [[ $# -ge 2 ]] || usage
        minutes=$2
        shift 2
        ;;
      -p)
        [[ $# -ge 2 ]] || usage
        prompts+=("$2")
        shift 2
        ;;
      *)
        prompts+=("$*")
        break
        ;;
    esac
  done
  [[ ${#prompts[@]} -gt 0 && -n ${prompts[0]} ]] || usage
  model_line "$model" >/dev/null || die "no model $model"
  for i in "${!prompts[@]}"; do
    runs[i]=$(new_run "${prompts[i]}" "$model" batch)
    status[i]=queued
  done
  for ((n = 0; n < slots; n++)); do
    slot_run[n]=-1
  done
  while [[ $done_count -lt ${#runs[@]} ]]; do
    for ((n = 0; n < slots; n++)); do
      i=${slot_run[n]}
      if [[ $i -ge 0 ]]; then
        if ! running "$n"; then
          status[i]="done"
          slot_run[n]=-1
          done_count=$((done_count + 1))
        elif [[ $SECONDS -ge ${deadline[i]} ]]; then
          systemctl stop "microvm@digger-$n.service"
          status[i]="stopped after $minutes min"
          slot_run[n]=-1
          done_count=$((done_count + 1))
        fi
      fi
      if [[ ${slot_run[n]} -lt 0 && $next -lt ${#runs[@]} ]] && ! running "$n"; then
        i=$next
        next=$((next + 1))
        printf '%s\n' "${runs[i]}" >"/run/digger/$n/target"
        if systemctl start "microvm@digger-$n.service"; then
          slot_run[n]=$i
          started[i]=$SECONDS
          deadline[i]=$((SECONDS + minutes * 60))
          status[i]=running
          echo "digger: $(basename "${runs[i]}") started on slot $n" >&2
        else
          status[i]="failed to start"
          done_count=$((done_count + 1))
        fi
      fi
    done
    [[ $done_count -lt ${#runs[@]} ]] && sleep 5
  done
  for i in "${!runs[@]}"; do
    result "${runs[i]}" "${status[i]}, $(( (SECONDS - ${started[i]:-$SECONDS}) / 60 )) min"
  done
}

cmd_stop() {
  local run slot
  [[ $# -ge 1 ]] || usage
  run=$(resolve "$1")
  slot=$(slot_of "$run") || die "$(basename "$run") is not running"
  systemctl stop "microvm@digger-$slot.service"
}

kb_finished() {
  local run=$1
  [[ -f $run/.kb/mem.db ]] || die "$(basename "$run") has no knowledge base copy"
  if slot_of "$run" >/dev/null; then
    die "$(basename "$run") is still running"
  fi
}

kb_record() {
  local run=$1 outcome=$2 note=$3 name key title body files
  name=$(basename "$run")
  key=find_$name
  title="Dig $(cut -c1-10 <<<"$name"): $(head -n 1 "$run/prompt.md" | cut -c1-100)"
  files=" none"
  if [[ -s $run/files/SOURCES.tsv ]]; then
    files=$'\n'$(awk -F'\t' 'NR <= 30 { n = split($2, p, "/"); u = ($6 != "" && $6 != $5) ? $5 " -> " $6 : $5; printf "- %s sha256 %s, %s bytes, %s\n", p[n], $3, $4, u }' "$run/files/SOURCES.tsv")
  fi
  body="Task: $(head -c 2000 "$run/prompt.md")
Outcome, judged by the orchestrator: $outcome.${note:+ $note}
Model: $(env_value "$run" MODEL). Run directory: $run.
Files fetched:$files"
  if kbmem get "$key" >/dev/null 2>&1; then
    kbmem put "$key" --update --title "$title" --body "$body" >/dev/null
  else
    kbmem put "$key" --kind fact --scope global --title "$title" --no-gate --why "dig record" --body "$body" >/dev/null
  fi
  echo "recorded $key"
}

kb_review_args() {
  outcome=unverified
  note=""
  while [[ ${1:-} == -* ]]; do
    case $1 in
      -o)
        [[ $# -ge 2 ]] || usage
        case $2 in
          found | partial | not-found | unverified) outcome=$2 ;;
          *) die "outcome is found, partial, not-found or unverified" ;;
        esac
        shift 2
        ;;
      -n)
        [[ $# -ge 2 ]] || usage
        note=$2
        shift 2
        ;;
      *) usage ;;
    esac
  done
  rest=("$@")
}

cmd_kb() {
  local run outcome note rest=() keys rc additions
  case ${1:-} in
    search)
      shift
      kb_init
      kbmem search "$@"
      ;;
    pending)
      runs | tac | while read -r run; do
        [[ -f $run/.kb/mem.db && ! -e $run/.kb/reviewed ]] || continue
        slot_of "$run" >/dev/null && continue
        additions=$(kb_additions "$run")
        if [[ -n $additions ]]; then
          echo "== $(basename "$run")"
          printf '%s\n' "$additions"
        fi
      done
      ;;
    show)
      [[ $# -ge 2 ]] || usage
      run=$(resolve "$2")
      kb_finished "$run"
      additions=$(kb_additions "$run")
      [[ -n $additions ]] || {
        echo "$(basename "$run") added nothing"
        return 0
      }
      printf '%s\n\n' "$additions"
      mapfile -t keys < <(cut -f2 <<<"$additions")
      runmem "$run" get "${keys[@]}"
      ;;
    accept | reject)
      local verb=$1
      shift
      kb_review_args "$@"
      [[ ${#rest[@]} -ge 1 ]] || usage
      run=$(resolve "${rest[0]}")
      kb_finished "$run"
      [[ ! -e $run/.kb/reviewed ]] || die "$(basename "$run") was already reviewed: $(cat "$run/.kb/reviewed")"
      kb_init
      rc=0
      if [[ $verb == accept ]]; then
        local only=() out
        if [[ ${#rest[@]} -gt 1 ]]; then
          only=(--key "$(IFS=,; printf '%s' "${rest[*]:1}")")
        fi
        sqlite3 -cmd '.timeout 10000' "$run/.kb/mem.db" 'PRAGMA wal_checkpoint(TRUNCATE)' >/dev/null
        out=$(kbmem merge --from "$run/.kb/mem.db" --since "$(cat "$run/.kb/base")" --source "digger:$(basename "$run")" \
          --why "accepted from $(basename "$run")" "${only[@]}") || rc=$?
        grep -v '^same' <<<"$out" || true
        [[ $rc == 0 || $rc == 3 ]] || die "the lines marked failed were not merged; fix them and run accept again"
      fi
      kb_record "$run" "$outcome" "$note"
      printf '%s %s\n' "${verb}ed" "$(date -u +%Y-%m-%dT%H:%M:%SZ)" >"$run/.kb/reviewed"
      [[ $rc == 0 ]] || echo "digger: the conflict lines duplicate records already there; fold them in by hand with digger kb mem put KEY --update" >&2
      ;;
    mem)
      shift
      kb_init
      kbmem "$@"
      ;;
    *) usage ;;
  esac
}

case ${1:-} in
  "" | -h | --help) usage ;;
  kb) shift && cmd_kb "$@" ;;
  attach) shift && cmd_attach "$@" ;;
  run) shift && cmd_run "$@" ;;
  resume) shift && cmd_resume "$@" ;;
  log) shift && cmd_log "$@" ;;
  list) cmd_list ;;
  stop) shift && cmd_stop "$@" ;;
  selftest) cmd_selftest ;;
  *) cmd_new "$@" ;;
esac
