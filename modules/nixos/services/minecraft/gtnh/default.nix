{
  config,
  options,
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
  inherit (inputs.nix-minecraft.lib) collectFilesAt;
  cfg = config.${namespace}.services.minecraft.gtnh;

  stdin = config.services.minecraft-servers.managementSystem.systemd-socket.stdinSocket.path "gtnh";
  systemctl = "${config.systemd.package}/bin/systemctl";
  journalctl = "${config.systemd.package}/bin/journalctl";
  saveOff = ''
    if ${systemctl} is-active --quiet minecraft-server-gtnh; then
      since=$(date '+%Y-%m-%d %H:%M:%S')
      echo 'save-off' > ${stdin}
      echo 'save-all flush' > ${stdin}
      timeout 600 grep -qm1 'Saved the world' \
        < <(${journalctl} -u minecraft-server-gtnh --since="$since" -f -o cat)
    fi
  '';
  saveOn = ''
    if ${systemctl} is-active --quiet minecraft-server-gtnh; then
      echo 'save-on' > ${stdin}
    fi
  '';

  serverPackage = pkgs.${namespace}.gtnh-server;

  configEntries = lib.mapAttrs' (
    name: _: lib.nameValuePair "config/${name}" "${serverPackage}/config/${name}"
  ) (builtins.readDir "${serverPackage}/config");

  levelName = "World";
  mapPort = config.${namespace}.ports.gtnh-map;

  webMap = pkgs.fetchurl {
    url = "https://github.com/GTNewHorizons/GTNH-Web-Map/releases/download/0.4-beta-1/gtnh-web-map-0.4-beta-1.jar";
    hash = "sha256-MfNpXlQzcGOpbOkGnzgo6tnUWDebt+R0EAv1zsaBRE0=";
  };

  extraMods = [
    webMap
    (pkgs.fetchurl {
      url = "https://github.com/FalsePattern/LagGogglesLegacy/releases/download/4.17.0/laggoggles-mc1.7.10-4.17.0.jar";
      hash = "sha256-TJg7E1LlMIxfV47ot7J6scSrjSLAJLDz+mLpirY0kyU=";
    })
    (pkgs.fetchurl {
      url = "https://github.com/GTNewHorizons/spark/releases/download/v0.0.16/spark-forge1710-1.10-SNAPSHOT.jar";
      hash = "sha256-NsC8fFZl29KvOE82CeBoQtPBSlK13692MiiPjwpXMU8=";
    })
  ];

  webMapConfig =
    pkgs.runCommand "gtnh-web-map-configuration.txt" { nativeBuildInputs = [ pkgs.unzip ]; }
      ''
        unzip -p ${webMap} configuration.txt > configuration.txt
        substitute configuration.txt $out \
          --replace-fail '#webserver-bindaddress: 0.0.0.0' 'webserver-bindaddress: 127.0.0.1' \
          --replace-fail 'webserver-port: 8123' 'webserver-port: ${toString mapPort}' \
          --replace-fail 'defaultworld: world' 'defaultworld: ${levelName}' \
          --replace-fail 'tiles-rendered-at-once: 2' 'tiles-rendered-at-once: 8' \
          --replace-fail '#parallelrendercnt: 4' 'parallelrendercnt: 8' \
          --replace-fail 'renderaccelerateinterval: 0.2' 'renderaccelerateinterval: 0.005'
      '';

  replaceFlags =
    replacements:
    lib.concatStringsSep " " (
      lib.mapAttrsToList (
        from: to: "--replace-fail ${lib.escapeShellArg from} ${lib.escapeShellArg to}"
      ) replacements
    );

  patchPackFile =
    path: replacements:
    pkgs.runCommand (baseNameOf path) { } ''
      substitute ${serverPackage}/${path} $out ${replaceFlags replacements}
    '';

  patchPackDir =
    dir: file: replacements:
    pkgs.runCommand (baseNameOf dir) { } ''
      cp -r --no-preserve=mode ${serverPackage}/${dir} $out
      substituteInPlace $out/${file} ${replaceFlags replacements}
    '';

  serverUtilitiesConfig = pkgs.runCommand "serverutilities.cfg" { } ''
    substitute ${serverPackage}/serverutilities/serverutilities.cfg $out ${
      replaceFlags {
        "B:back=false" = "B:back=true";
        "B:home=false" = "B:home=true";
        "B:spawn=false" = "B:spawn=true";
        "B:tpa=false" = "B:tpa=true";
        "B:chunk_claiming=false" = "B:chunk_claiming=true";
        "S:enable_explosions=TRUE" = "S:enable_explosions=DEFAULT";
        "S:enable_pvp=TRUE" = "S:enable_pvp=DEFAULT";
      }
    }
    cat >> $out <<'EOF'
    pregen {
        I:chunksPerTick=50
        S:timeLimitMs=30.0
    }

    tab {
        S:footerText=
        S:headerText=&6GregTech: New Horizons ${serverPackage.version}
    }
    EOF
  '';

  configOverrides = {
    "config/EnderStorage.cfg" = patchPackFile "config/EnderStorage.cfg" {
      "item.storage-size=1" = "item.storage-size=2";
    };
    "config/forestry" = patchPackDir "config/forestry" "common.cfg" {
      "B:disable.butterfly=false" = "B:disable.butterfly=true";
    };
    "config/GregTech" = patchPackDir "config/GregTech" "Pollution.cfg" {
      "B:\"Activate Pollution\"=true" = "B:\"Activate Pollution\"=false";
    };
    "config/RWG.cfg" = patchPackFile "config/RWG.cfg" {
      "B:\"Generate Underground Lakes\"=true" = "B:\"Generate Underground Lakes\"=false";
    };
    "serverutilities/server/ranks.txt" = patchPackFile "serverutilities/server/ranks.txt" {
      "serverutilities.homes.max: 1" = "serverutilities.homes.max: 5";
    };
    "serverutilities/serverutilities.cfg" = serverUtilitiesConfig;
    "config/structurelib.cfg" = pkgs.writeText "structurelib.cfg" ''
      ~CONFIG_VERSION: V1

      common {
          hologram {
              I:autoPlaceBudget=200
              I:autoPlaceInterval=50
          }
      }
    '';
  };

  jvmOpts = lib.concatStringsSep " " [
    "-Xmx10G"
    "-Xms4G"
    "-XX:+UnlockExperimentalVMOptions"
    "-XX:+UnlockDiagnosticVMOptions"
    "-XX:+UseTransparentHugePages"
    "-XX:+DisableExplicitGC"
    "-XX:+UseStringDeduplication"
    "-XX:+UseCriticalJavaThreadPriority"
    "-XX:+AlwaysActAsServerClassMachine"
    "-XX:+DebugNonSafepoints"
    "-XX:+UseCompactObjectHeaders"
    "-XX:CICompilerCount=6"
    "-XX:AllocatePrefetchStyle=3"
    "-XX:-DontCompileHugeMethods"
    "-XX:ReservedCodeCacheSize=400M"
    "-XX:NonNMethodCodeHeapSize=12M"
    "-XX:ProfiledCodeHeapSize=194M"
    "-XX:NonProfiledCodeHeapSize=194M"
    "-XX:+UseVectorStubs"
    "-XX:TrimNativeHeapInterval=5000"
    "-XX:+UseZGC"
    "-Djdk.graal.OptimizeLongJumps=true"
    "-Djdk.graal.TrivialInliningSize=16"
    "-Djdk.graal.MaximumInliningSize=320"
    "-Djdk.graal.MaximumEscapeAnalysisArrayLength=512"
    "-Djdk.graal.LoopRotation=true"
    "-Djdk.graal.TuneInlinerExploration=1"
    "-Djdk.graal.SpectrePHTBarriers=None"
    "-Djdk.graal.SpectrePHTIndexMasking=false"
    "-Djdk.nio.maxCachedBufferSize=262144"
    "-Dfml.readTimeout=180"
    "-Duser.language=en"
  ];
in
{
  options.${namespace}.services.minecraft.gtnh = {
    enable = mkEnableOption "running the GT New Horizons Minecraft server";
    retain = mkEnableOption "backing up its data while the service is off" // {
      default = options.${namespace}.services.minecraft.gtnh.enable.highestPrio < 1500;
      defaultText = "Whether this host sets `enable` at all, even to false.";
    };
    port = mkOption {
      type = types.port;
      default = 25565;
    };
  };

  config = mkMerge [
    (mkIf (cfg.enable || cfg.retain) {
      ${namespace} = {
        services.kopia.backup = [
          {
            path = "/srv/minecraft/gtnh";
            ignore = [ "/backups" ];
            preCommand = saveOff;
            postCommand = saveOn;
          }
        ];

        system.impermanence.directories = [
          {
            directory = "/srv/minecraft";
            user = "minecraft";
            group = "minecraft";
            mode = "0750";
          }
        ];
      };
    })
    (mkIf cfg.enable {
      services.minecraft-servers = {
        enable = true;
        eula = true;
        managementSystem = {
          tmux.enable = false;
          systemd-socket.enable = true;
        };
        servers.gtnh = {
          enable = true;
          autoStart = true;
          openFirewall = true;
          package = serverPackage;
          inherit jvmOpts;

          serverProperties = {
            server-port = cfg.port;
            motd = "Hosted by ElXreno";
            level-name = levelName;
            level-type = "rwg";
            difficulty = 3;
            white-list = true;
            allow-flight = true;
            enable-command-block = true;
            view-distance = 8;
            spawn-protection = 1;
          };

          whitelist = {
            ElXreno = "40601523-a29f-414e-a942-799b839423ec";
            kontonkara = "723f7324-2389-4ef8-bf8e-85390ef0fab1";
            G_R_O_M_I_L_A = "98f60092-8c67-4c3a-92a0-27b8ed10904c";
          };

          symlinks =
            collectFilesAt serverPackage "mods"
            // lib.listToAttrs (map (jar: lib.nameValuePair "mods/${jar.name}" jar) extraMods)
            // {
              "server-icon.png" = "${serverPackage}/server-icon.png";
            };

          files =
            configEntries
            // collectFilesAt serverPackage "serverutilities"
            // configOverrides
            // {
              "dynmap/configuration.txt" = webMapConfig;
            };
        };
      };

      services.caddy.virtualHosts."map.elxreno.com".extraConfig = ''
        reverse_proxy 127.0.0.1:${toString mapPort}
      '';

      ${namespace} = {
        ports = {
          gtnh = cfg.port;
          gtnh-map = 8123;
        };

        services.caddy.enable = true;

        system.networking = {
          dns.records."map.elxreno.com".proxied = true;
          firewall.public.minecraft = {
            upstream = "modules/minecraft-servers.nix";
            tcp = [ cfg.port ];
          };
        };
      };

      systemd.services.minecraft-server-gtnh.serviceConfig = {
        TimeoutStopSec = lib.mkForce "10min";
      };
    })
  ];
}
