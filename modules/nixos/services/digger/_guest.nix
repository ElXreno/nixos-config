{
  cfg,
  namespace,
  subnet,
  guestUid,
  bridges,
  stateVersion,
}:
{
  lib,
  pkgs,
  ...
}:
let
  inherit (lib) getExe;

  localPorts = lib.listToAttrs (map (b: lib.nameValuePair b.name b.guestPort) bridges);

  tool =
    name: runtimeInputs:
    pkgs.writeShellApplication {
      inherit name runtimeInputs;
      text = builtins.readFile ./tools/${name}.sh;
    };

  grab = tool "grab" (
    with pkgs;
    [
      coreutils
      curl
      file
      gnused
      binutils
    ]
  );

  tools = [
    grab
    (tool "websearch" (
      with pkgs;
      [
        curl
        jq
      ]
    ))
    (tool "wayback" (
      with pkgs;
      [
        curl
        jq
        grab
      ]
    ))
    (tool "textify" (
      with pkgs;
      [
        coreutils
        file
        poppler-utils
        ocrmypdf
        tesseract
        python3Packages.trafilatura
        html2text
        p7zip
        unar
        libreoffice
        exiftool
        binwalk
        binutils
      ]
    ))
  ];

  python = pkgs.python3.withPackages (
    ps: with ps; [
      requests
      beautifulsoup4
      lxml
      pypdf
      trafilatura
      pefile
      lief
      capstone
      pyelftools
    ]
  );

  rea = pkgs.${namespace}.rea-full;

  reaMcp = pkgs.writeText "digger-rea-mcp.json" (
    builtins.toJSON {
      mcpServers.rea = {
        command = lib.getExe rea;
        args = [ "mcp" ];
        env.REA_GHIDRA_CHECKPOINT_DIR = "/work/.rea/ghidra-checkpoints";
      };
    }
  );

  reverse = with pkgs; [
    rea
    (rizin.withPlugins (ps: [ ps.rz-ghidra ]))
    ghidra
    jadx
    apktool
    ilspycmd
    unblob
    upx
    checksec
    yara
    dtc
    gdb
    wabt
    wireshark-cli
  ];

  packages =
    tools
    ++ [ python ]
    ++ reverse
    ++ [
      pkgs.${namespace}.camoufox-cli
      pkgs.${namespace}.mem
    ]
    ++ (with pkgs; [
      claude-code
      coreutils
      findutils
      which
      util-linux
      iputils
      netcat-openbsd
      tree
      bc
      gnugrep
      gnused
      gawk
      diffutils
      less
      procps
      curl
      wget
      aria2
      yt-dlp
      gallery-dl
      megatools
      gdown
      python3Packages.internetarchive
      git
      gh
      jq
      yq-go
      htmlq
      pup
      xmlstarlet
      ripgrep
      fd
      poppler-utils
      qpdf
      mupdf
      ocrmypdf
      tesseract
      pandoc
      libreoffice
      p7zip
      unar
      unzip
      zip
      gzip
      bzip2
      xz
      zstd
      cabextract
      innoextract
      binwalk
      squashfsTools
      sasquatch
      file
      binutils-unwrapped-all-targets
      xxd
      hexyl
      exiftool
      sqlite
      w3m
      lynx
      dnsutils
      whois
      openssl
      gnupg
      minisign
      signify
    ]);

  memHook = event: args: {
    type = "command";
    command = lib.escapeShellArgs (
      [
        (getExe pkgs.${namespace}.mem)
        "hook"
        event
      ]
      ++ args
    );
    timeout = 5;
  };

  settings = pkgs.writeText "digger-claude-settings.json" (
    builtins.toJSON {
      skipDangerousModePermissionPrompt = true;
      hooks = {
        SessionStart = [
          {
            matcher = "startup|resume|clear|compact";
            hooks = [ (memHook "session-start" [ "--no-usage" ]) ];
          }
        ];
        UserPromptSubmit = [ { hooks = [ (memHook "user-prompt-submit" [ ]) ]; } ];
        PreToolUse = [
          {
            matcher = "*";
            hooks = [
              (memHook "pre-tool-use" [
                "--budget"
                "3s"
                "--agent-recall"
              ])
            ];
          }
        ];
        SubagentStart = [ { hooks = [ (memHook "subagent-start" [ ]) ]; } ];
      };
    }
  );

  selftest = pkgs.writeShellApplication {
    name = "digger-selftest";
    runtimeInputs = packages ++ [
      pkgs.bash
      pkgs.iproute2
    ];
    text = ''
      reachable() {
        timeout 5 bash -c "</dev/tcp/$1/$2" 2>/dev/null
      }
      check() {
        local name=$1
        shift
        if "$@" >/dev/null 2>&1; then echo "ok    $name"; else echo "FAIL  $name"; fi
      }
      deny() {
        if reachable "$2" "$3"; then echo "FAIL  $1 $2:$3 is reachable"; else echo "ok    $1 $2:$3 blocked"; fi
      }

      gw=$(ip route show default | cut -d' ' -f3)
      {
        check "dns" dig +short +time=5 example.com A
        check "https" curl -sf -m 15 -o /dev/null https://example.com
        check "gateway models" curl -sf -m 10 "$ANTHROPIC_BASE_URL/v1/models"
        check "searxng" curl -sf -m 60 -o /dev/null "$SEARXNG_URL/search?q=binwalk&format=json"
        check "control 1.1.1.1:443" reachable 1.1.1.1 443
        deny "host" "$gw" 22
        deny "host" "$gw" ${toString localPorts.gateway}
        deny "link-local" 169.254.169.254 80
        while read -r label ip port; do
          [[ -n $label ]] && deny "$label" "$ip" "$port"
        done </work/selftest
        for t in claude mem camoufox-cli websearch wayback grab textify 7z unar binwalk pdftotext tesseract soffice python3 yt-dlp megadl gdown ia git rg rizin ghidra-analyzeHeadless jadx apktool unblob rea tmux; do
          check "tool $t" command -v "$t"
        done
        check "work writable" touch /work/.selftest-write
        rm -f /work/.selftest-write
        check "kb write" env MEM_DB=/work/.kb-selftest/mem.db MEM_CORE=/work/.kb-selftest/core.md mem put selftest_probe --kind fact --scope global --title "Selftest probe" --body "selftest probe record"
        check "kb search" bash -c 'MEM_DB=/work/.kb-selftest/mem.db mem search probe | grep -q selftest_probe'
        rm -rf /work/.kb-selftest
        check "camoufox open" timeout 180 camoufox-cli --session selftest open https://example.com
        check "camoufox title" bash -c 'timeout 60 camoufox-cli --raw --session selftest eval "document.title" | grep -q "Example Domain"'
        check "camoufox snapshot" bash -c 'timeout 60 camoufox-cli --session selftest snapshot | grep -q "ref=e"'
        timeout 60 camoufox-cli --session selftest close >/dev/null 2>&1 || true
      } >/work/selftest.txt 2>&1
    '';
  };

  globalConfig = pkgs.writeText "digger-claude-global.json" (
    builtins.toJSON {
      hasCompletedOnboarding = true;
      bypassPermissionsModeAccepted = true;
      theme = "dark";
      projects."/work" = {
        hasTrustDialogAccepted = true;
        hasCompletedProjectOnboarding = true;
      };
    }
  );

  dig = pkgs.writeShellApplication {
    name = "digger-claude";
    runtimeInputs = packages;
    text = ''
      cd /work
      MODEL=
      CTX=
      TOOL_SEARCH=
      MODE=
      BASE_PORT=${toString localPorts.gateway}
      # shellcheck source=/dev/null
      . /work/run.env
      export ANTHROPIC_BASE_URL=http://127.0.0.1:$BASE_PORT

      export CLAUDE_CONFIG_DIR=/work/.claude
      mkdir -p "$CLAUDE_CONFIG_DIR/skills" "$CLAUDE_CONFIG_DIR/agents" /work/files /work/leads
      install -m 0644 ${./CLAUDE.md} "$CLAUDE_CONFIG_DIR/CLAUDE.md"
      install -m 0644 ${./lead.md} "$CLAUDE_CONFIG_DIR/agents/lead.md"
      install -m 0644 ${settings} "$CLAUDE_CONFIG_DIR/settings.json"
      [[ -f $CLAUDE_CONFIG_DIR/.claude.json ]] || install -m 0644 ${globalConfig} "$CLAUDE_CONFIG_DIR/.claude.json"
      ln -sfn ${./skill} "$CLAUDE_CONFIG_DIR/skills/dig"
      rm -f "$CLAUDE_CONFIG_DIR/skills/agent-browser"
      ln -sfn ${pkgs.${namespace}.camoufox-cli}/share/camoufox-cli/skills/camoufox "$CLAUDE_CONFIG_DIR/skills/camoufox"

      mcp=()
      if [[ $TOOL_SEARCH == true ]]; then
        mkdir -p /work/.rea/ghidra-checkpoints
        ln -sfn ${rea.skill} "$CLAUDE_CONFIG_DIR/skills/reverse-engineer-anything"
        mcp=(--mcp-config ${reaMcp})
      else
        rm -f "$CLAUDE_CONFIG_DIR/skills/reverse-engineer-anything"
      fi

      export ANTHROPIC_MODEL=$MODEL
      export ANTHROPIC_DEFAULT_OPUS_MODEL=$MODEL
      export ANTHROPIC_DEFAULT_SONNET_MODEL=$MODEL
      export ANTHROPIC_DEFAULT_FABLE_MODEL=$MODEL
      export CLAUDE_CODE_MAX_CONTEXT_TOKENS=$CTX
      export ENABLE_TOOL_SEARCH=$TOOL_SEARCH

      args=(--model "$MODEL" --effort max --permission-mode bypassPermissions "''${mcp[@]}")
      if [[ -f followup.md ]]; then
        prompt=$(cat followup.md)
        mkdir -p followups
        mv followup.md "followups/$(date -u +%Y%m%dT%H%M%SZ).md"
        args+=(--continue)
      else
        prompt=$(cat prompt.md)
      fi

      if [[ $MODE == batch ]]; then
        printf '%s\n' "$prompt" |
          claude -p --verbose --output-format stream-json "''${args[@]}" --disallowedTools WebSearch >>agent.jsonl 2>>agent.err
      elif [[ -n $prompt ]]; then
        exec claude "$prompt" "''${args[@]}" --disallowedTools WebSearch
      else
        exec claude "''${args[@]}" --disallowedTools WebSearch
      fi
    '';
  };

  agent = pkgs.writeShellApplication {
    name = "digger-agent";
    runtimeInputs = [
      pkgs.coreutils
      pkgs.gnugrep
      pkgs.tmux
    ];
    text = ''
      cd /work
      if [[ -f selftest ]]; then
        exec ${getExe selftest}
      fi
      if grep -qx 'MODE=batch' run.env; then
        exec ${getExe dig}
      fi
      tmux -S /tmp/digger.tmux new-session -d -s dig -x 200 -y 50 -c /work ${getExe dig}
      while tmux -S /tmp/digger.tmux has-session -t dig 2>/dev/null; do
        sleep 2
      done
    '';
  };
in
{
  system.stateVersion = stateVersion;
  networking.hostName = "digger";

  microvm = {
    hypervisor = "cloud-hypervisor";
    inherit (cfg) vcpu;
    mem = cfg.memory;
    registerClosure = false;
    shares = [
      {
        tag = "work";
        proto = "virtiofs";
        source = "work";
        socket = "work.sock";
        mountPoint = "/work";
      }
    ];
  };

  nix.enable = false;

  networking = {
    useDHCP = false;
    nameservers = [
      "1.1.1.1"
      "9.9.9.9"
    ];
  };
  services.resolved.enable = false;

  users.users.digger = {
    isNormalUser = true;
    uid = guestUid;
  };

  microvm.vsock.ssh.enable = true;
  services.openssh = {
    openFirewall = false;
    listenAddresses = [
      {
        addr = "127.0.0.1";
        port = 22;
      }
    ];
    authorizedKeysFiles = [ "/work/.digger/authorized_keys" ];
    settings = {
      PasswordAuthentication = false;
      KbdInteractiveAuthentication = false;
      PermitRootLogin = "no";
      AllowUsers = [ "digger" ];
      X11Forwarding = false;
      AllowAgentForwarding = false;
      AllowTcpForwarding = false;
      AllowStreamLocalForwarding = false;
    };
  };

  programs.tmux = {
    enable = true;
    escapeTime = 0;
    historyLimit = 100000;
    terminal = "tmux-256color";
    extraConfig = ''
      set -as terminal-features ",*:RGB"
      set -g status off
      set -g set-clipboard off
      set -g allow-passthrough off
    '';
  };

  environment.enableAllTerminfo = true;
  environment.systemPackages = packages;

  systemd.services = {
    digger-net = {
      wantedBy = [ "network-online.target" ];
      before = [ "network-online.target" ];
      path = with pkgs; [
        coreutils
        iproute2
      ];
      serviceConfig = {
        Type = "oneshot";
        RemainAfterExit = true;
      };
      script = ''
        for _ in $(seq 300); do
          for dev in /sys/class/net/*; do
            name=''${dev##*/}
            [ "$name" = lo ] && continue
            mac=$(cat "$dev/address")
            slot=$((16#''${mac##*:}))
            ip link set "$name" up
            ip addr add ${subnet}.$slot.2/24 dev "$name"
            ip route add default via ${subnet}.$slot.1
            exit 0
          done
          sleep 0.1
        done
        echo "digger-net: no network interface" >&2
        exit 1
      '';
    };

    digger-agent = {
      wantedBy = [ "multi-user.target" ];
      wants = [ "network-online.target" ];
      after = [ "network-online.target" ] ++ map (b: "digger-vsock-${b.name}.service") bridges;
      unitConfig = {
        RequiresMountsFor = "/work";
        SuccessAction = "poweroff";
        FailureAction = "poweroff";
      };
      environment = {
        HOME = "/home/digger";
        ANTHROPIC_BASE_URL = "http://127.0.0.1:${toString localPorts.gateway}";
        ANTHROPIC_API_KEY = "";
        ANTHROPIC_AUTH_TOKEN = "local";
        CLAUDE_CODE_SUBAGENT_MODEL_FORCE = "1";
        CLAUDE_CODE_NO_MODEL_FALLBACK = "1";
        CLAUDE_CODE_MAX_OUTPUT_TOKENS = "32768";
        CLAUDE_CODE_ATTRIBUTION_HEADER = "0";
        CLAUDE_CODE_DISABLE_NONESSENTIAL_TRAFFIC = "1";
        DISABLE_AUTOUPDATER = "1";
        API_TIMEOUT_MS = "3600000";
        CLAUDE_STREAM_IDLE_TIMEOUT_MS = "3600000";
        SEARXNG_URL = "http://127.0.0.1:${toString localPorts.searxng}";
        MEM_DB = "/work/.kb/mem.db";
        MEM_CORE = "/work/.kb/core.md";
        COLORTERM = "truecolor";
      };
      serviceConfig = {
        Type = "exec";
        User = "digger";
        StandardError = "journal+console";
        WorkingDirectory = "/work";
        ExecStart = getExe agent;
      };
    };
  }
  // lib.listToAttrs (
    map (
      b:
      lib.nameValuePair "digger-vsock-${b.name}" {
        wantedBy = [ "multi-user.target" ];
        before = [ "digger-agent.service" ];
        serviceConfig = {
          ExecStart = "${getExe pkgs.socat} TCP-LISTEN:${toString b.guestPort},bind=127.0.0.1,reuseaddr,fork VSOCK-CONNECT:2:${toString b.vsock}";
          DynamicUser = true;
          Restart = "always";
        };
      }
    ) bridges
  );
}
