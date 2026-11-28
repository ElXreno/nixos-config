{
  config,
  inputs,
  namespace,
  lib,
  pkgs,
  ...
}:
let
  inherit (lib)
    mkIf
    mkMerge
    mkEnableOption
    mkOption
    types
    ;
  cfg = config.${namespace}.services.digger;
  services = config.${namespace}.services;
  user = config.${namespace}.primaryUser;
  inherit (config.microvm) stateDir;

  subnet = "10.231";
  owner = config.users.users.${user};
  guestUid = owner.uid;
  remoteNames = lib.attrNames cfg.remotes;
  bridges = [
    {
      name = "gateway";
      vsock = 9001;
      guestPort = services.llm-gateway.port;
      target = "127.0.0.1:${toString services.llm-gateway.port}";
    }
    {
      name = "searxng";
      vsock = 9002;
      guestPort = services.searxng.port;
      target = "127.0.0.1:${toString services.searxng.port}";
    }
  ]
  ++ lib.imap0 (i: r: {
    name = "remote-${r}";
    vsock = 9003 + i;
    guestPort = 8190 + i;
    target = cfg.remotes.${r}.address;
  }) remoteNames;
  bridge = name: lib.findFirst (b: b.name == name) null bridges;

  slots = lib.genList lib.id cfg.slots;
  vmName = n: "digger-${toString n}";
  tap = n: "dg${toString n}";
  mac = n: "02:00:00:d1:99:${lib.fixedWidthString 2 "0" (lib.toHexString n)}";
  forSlots = f: lib.listToAttrs (map f slots);
  forSlotBridges = f: lib.listToAttrs (lib.concatMap (n: map (f n) bridges) slots);

  peers = lib.listToAttrs (map (m: lib.nameValuePair m.peer m) services.llm-gateway.models);

  guest = import ./_guest.nix {
    inherit
      cfg
      namespace
      subnet
      guestUid
      bridges
      ;
    inherit (config.system) stateVersion;
  };

  workd = pkgs.writeShellApplication {
    name = "digger-workd";
    runtimeInputs = [
      pkgs.coreutils
      pkgs.virtiofsd
    ];
    text = ''
      slot=$1
      run=$(realpath -e -- "$(cat "/run/digger/$slot/target")")
      case $run in
        ${cfg.runRoot}/*/*)
          echo "digger-workd: $run is not a run directory" >&2
          exit 1
          ;;
        ${cfg.runRoot}/?*) ;;
        *)
          echo "digger-workd: $run is outside ${cfg.runRoot}" >&2
          exit 1
          ;;
      esac
      exec virtiofsd --socket-path=work.sock --socket-group=kvm --shared-dir="$run" --cache=auto
    '';
  };

  waitSocket =
    socket:
    pkgs.writeShellScript "digger-wait-${socket}" ''
      for _ in $(${pkgs.coreutils}/bin/seq 100); do
        [ -S ${socket} ] && exit 0
        ${pkgs.coreutils}/bin/sleep 0.1
      done
      exit 1
    '';

  storeView = "/run/digger-store";

  guestClosure =
    n:
    pkgs.closureInfo {
      rootPaths = [ config.microvm.vms.${vmName n}.config.config.system.build.toplevel ];
    };

  stored = pkgs.writeShellApplication {
    name = "digger-stored";
    runtimeInputs = [
      pkgs.coreutils
      pkgs.util-linux
      pkgs.virtiofsd
    ];
    text = ''
      mount -t tmpfs -o mode=0755 digger-store ${storeView}
      while IFS= read -r path; do
        name=''${path#/nix/store/}
        if [ -L "$path" ]; then
          cp -P "$path" "${storeView}/$name"
          continue
        fi
        if [ -d "$path" ]; then
          mkdir "${storeView}/$name"
        else
          touch "${storeView}/$name"
        fi
        mount --bind -o ro "$path" "${storeView}/$name"
      done < "$1"
      exec virtiofsd --socket-path=store.sock --socket-group=kvm --shared-dir=${storeView} \
        --readonly --cache=always --no-announce-submounts
    '';
  };

  shareVsock = pkgs.writeShellScript "digger-share-vsock" ''
    for _ in $(${pkgs.coreutils}/bin/seq 100); do
      if [ -S notify.vsock ]; then
        exec ${pkgs.coreutils}/bin/chmod 0660 notify.vsock
      fi
      ${pkgs.coreutils}/bin/sleep 0.1
    done
    exit 1
  '';

  modelTable =
    lib.concatMapStrings (
      m:
      "${m.peer}\t${m.model}\t${toString m.ctxSize}\t${lib.boolToString m.toolSearch}\t${toString (bridge "gateway").guestPort}\n"
    ) services.llm-gateway.models
    + lib.concatMapStrings (
      r:
      let
        remote = cfg.remotes.${r};
      in
      "${r}\t${remote.model}\t${toString remote.ctxSize}\t${lib.boolToString remote.toolSearch}\t${toString (bridge "remote-${r}").guestPort}\n"
    ) remoteNames;

  cli = pkgs.writeShellApplication {
    name = "digger";
    runtimeInputs = with pkgs; [
      bash
      coreutils
      gnugrep
      gnused
      iproute2
      jq
      openssh
      sqlite
      config.systemd.package
    ];
    text = ''
      mem=${lib.getExe pkgs.${namespace}.mem}
      runRoot=${lib.escapeShellArg cfg.runRoot}
      stateDir=${lib.escapeShellArg stateDir}
      sshProxy=${config.systemd.package}/lib/systemd/systemd-ssh-proxy
      slots=${toString cfg.slots}
      defaultPeer=${lib.escapeShellArg cfg.peer}
      models=${lib.escapeShellArg modelTable}
      format=${./format.jq}
    ''
    + builtins.readFile ./digger.sh;
  };
in
{
  imports = [ inputs.microvm.nixosModules.host ];

  options.${namespace}.services.digger = {
    enable = mkEnableOption "web digger agents in disposable microVMs";

    slots = mkOption {
      type = types.ints.between 1 16;
      default = 2;
      description = "Digger VMs that may run at the same time.";
    };

    peer = mkOption {
      type = types.enum (lib.attrNames peers ++ remoteNames);
      default = (lib.head services.llm-gateway.models).peer;
      defaultText = lib.literalExpression "the first model the llm-gateway routes";
      description = "llm-gateway peer or remote the agents use unless a run picks another.";
    };

    remotes = mkOption {
      type = types.attrsOf (
        types.submodule {
          options = {
            address = mkOption {
              type = types.str;
              example = "kurwa.example.ts.net:8097";
              description = "host:port of an Anthropic-compatible model server the host reaches; the VMs reach it only through vsock.";
            };
            model = mkOption {
              type = types.str;
              description = "Model id the server serves.";
            };
            ctxSize = mkOption {
              type = types.ints.positive;
              description = "Context window of the served model.";
            };
            toolSearch = mkEnableOption "Claude Code tool search against this server";
          };
        }
      );
      default = { };
      description = "Model servers outside the host's llm-gateway that runs may pick with -m <name>.";
    };

    vcpu = mkOption {
      type = types.ints.positive;
      default = 4;
      description = "Virtual CPUs of each VM.";
    };

    memory = mkOption {
      type = types.ints.positive;
      default = 8192;
      description = "RAM of each VM in MiB.";
    };

    runRoot = mkOption {
      type = types.str;
      default = "/mnt/scratch/digger";
      description = "Directory holding one subdirectory per run, shared read-write with the VM that runs it.";
    };
  };

  config = mkMerge [
    { microvm.host.enable = cfg.enable; }

    (mkIf cfg.enable {
      assertions = [
        {
          assertion = services.llm-gateway.enable && services.searxng.enable;
          message = "${namespace}.services.digger needs the llm-gateway and searxng services on the host";
        }
        {
          assertion = owner.uid != null && config.users.groups.${owner.group}.gid == config.ids.gids.users;
          message = "${namespace}.services.digger maps the run directory to the guest user by uid and the users group, so ${user} needs a fixed uid and users as the primary group";
        }
      ];

      hardware.ksm.enable = false;

      microvm.vms = forSlots (
        n:
        lib.nameValuePair (vmName n) {
          autostart = false;
          restartIfChanged = false;
          config = {
            imports = [ guest ];
            microvm = {
              interfaces = [
                {
                  type = "tap";
                  id = tap n;
                  mac = mac n;
                }
              ];
              vsock.cid = 3 + n;
            };
          };
        }
      );

      systemd.services =
        forSlots (
          n:
          lib.nameValuePair "microvm@${vmName n}" {
            requires = [ "${vmName n}-store.service" ];
            after = [ "${vmName n}-store.service" ];
            serviceConfig = {
              Type = lib.mkForce "simple";
              Restart = "no";
              UMask = "0007";
              ExecStartPost = [ "-${shareVsock}" ];
              ExecStop = [
                ""
                "-${stateDir}/%i/booted/bin/microvm-shutdown"
              ];
            };
          }
        )
        // forSlots (
          n:
          lib.nameValuePair "microvm-virtiofsd@${vmName n}" {
            serviceConfig = {
              Type = "simple";
              User = user;
              Group = owner.group;
              SupplementaryGroups = [ "kvm" ];
              ExecStart = [
                ""
                "${lib.getExe workd} ${toString n}"
              ];
              ExecStartPost = [ (waitSocket "work.sock") ];
              Restart = "no";
            };
          }
        )
        // forSlots (
          n:
          lib.nameValuePair "${vmName n}-store" {
            description = "Read-only store view of the ${vmName n} guest closure";
            serviceConfig = {
              Type = "simple";
              WorkingDirectory = "${stateDir}/${vmName n}";
              PrivateMounts = true;
              ExecStartPre = [ "${pkgs.coreutils}/bin/rm -f store.sock" ];
              ExecStart = "${lib.getExe stored} ${guestClosure n}/store-paths";
              ExecStartPost = [ (waitSocket "store.sock") ];
              Restart = "no";
            };
          }
        )
        // forSlotBridges (
          n: b:
          lib.nameValuePair "${vmName n}-${b.name}" {
            requires = [ "${vmName n}-${b.name}.socket" ];
            after = [ "${vmName n}-${b.name}.socket" ];
            serviceConfig = {
              ExecStart = "${config.systemd.package}/lib/systemd/systemd-socket-proxyd ${b.target}";
              DynamicUser = true;
              PrivateTmp = true;
              ProtectSystem = "strict";
              ProtectHome = true;
              NoNewPrivileges = true;
            };
          }
        );

      systemd.sockets = forSlotBridges (
        n: b:
        lib.nameValuePair "${vmName n}-${b.name}" {
          wantedBy = [ "sockets.target" ];
          listenStreams = [ "${stateDir}/${vmName n}/notify.vsock_${toString b.vsock}" ];
          socketConfig = {
            SocketUser = "microvm";
            SocketGroup = "kvm";
            SocketMode = "0660";
          };
        }
      );

      systemd.tmpfiles.settings."10-digger" = {
        ${cfg.runRoot}.d = {
          inherit user;
          group = "users";
          mode = "0750";
        };
        "/run/digger".d = {
          user = "root";
          group = "root";
          mode = "0755";
        };
        ${storeView}.d = {
          user = "root";
          group = "root";
          mode = "0755";
        };
      }
      // forSlots (
        n:
        lib.nameValuePair "/run/digger/${toString n}" {
          d = {
            inherit user;
            group = "users";
            mode = "0700";
          };
        }
      );

      systemd.network.networks = forSlots (
        n:
        lib.nameValuePair "40-${tap n}" {
          matchConfig.Name = tap n;
          address = [ "${subnet}.${toString n}.1/24" ];
          networkConfig = {
            LinkLocalAddressing = "no";
            IPv6AcceptRA = false;
            ConfigureWithoutCarrier = true;
          };
          linkConfig.RequiredForOnline = "no";
        }
      );

      networking.networkmanager.unmanaged = [ "interface-name:dg*" ];

      networking.nftables.tables.digger = {
        family = "inet";
        content = ''
          set blocked4 {
            type ipv4_addr
            flags interval
            elements = {
              0.0.0.0/8, 10.0.0.0/8, 100.64.0.0/10, 127.0.0.0/8, 169.254.0.0/16,
              172.16.0.0/12, 192.0.0.0/24, 192.168.0.0/16, 198.18.0.0/15, 224.0.0.0/3
            }
          }

          chain input {
            type filter hook input priority filter - 10; policy accept;
            iifname "dg*" ct state established,related accept
            iifname "dg*" drop
          }

          chain forward {
            type filter hook forward priority filter - 10; policy accept;
            iifname "dg*" meta nfproto ipv6 drop
            iifname "dg*" ip saddr != ${subnet}.0.0/16 drop
            iifname "dg*" oifname "dg*" drop
            iifname "dg*" ip daddr @blocked4 drop
            oifname "dg*" ct state established,related accept
            oifname "dg*" drop
          }

          chain postrouting {
            type nat hook postrouting priority srcnat; policy accept;
            ip saddr ${subnet}.0.0/16 oifname != "dg*" masquerade
          }
        '';
      };

      security.polkit.extraConfig = ''
        polkit.addRule(function(action, subject) {
          if (action.id == "org.freedesktop.systemd1.manage-units" &&
              subject.user == "${user}" &&
              /^microvm@digger-[0-9]+\.service$/.test(action.lookup("unit")) &&
              ["start", "stop"].indexOf(action.lookup("verb")) >= 0) {
            return polkit.Result.YES;
          }
        });
      '';

      environment.systemPackages = [ cli ];
    })
  ];
}
