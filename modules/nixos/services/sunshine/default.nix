{
  config,
  namespace,
  lib,
  pkgs,
  ...
}:

let
  inherit (lib)
    concatStringsSep
    getExe
    mkEnableOption
    mkForce
    mkIf
    mkOption
    optionalAttrs
    optionals
    toLower
    types
    ;
  cfg = config.${namespace}.services.sunshine;
  steam = config.${namespace}.programs.steam;

  basePort = 47989;
  webPort = basePort + 1;

  tailnetDomain = config.clan.core.settings.domain;
  serviceName = "sunshine";

  hostName = toLower config.networking.hostName;
  allowedOrigins = [
    "https://${hostName}"
    "https://${hostName}.${tailnetDomain}"
    "https://${serviceName}.${tailnetDomain}"
  ];

  setsid = "${pkgs.util-linux}/bin/setsid";
  steamBin = "${config.programs.steam.package}/bin/steam";

  configFile = (pkgs.formats.keyValue { }).generate "sunshine.conf" config.services.sunshine.settings;
  passwordFile = config.clan.core.vars.generators.sunshine.files.password.path;

  applyCredentials = pkgs.writeShellScript "sunshine-apply-credentials" ''
    exec ${getExe config.services.sunshine.package} ${configFile} \
      --creds ${cfg.user} "$(< ${passwordFile})"
  '';

  encoderGenerator = pkgs.writeShellScript "sunshine-encoder" ''
    for connector in /sys/class/drm/card*-eDP-*; do
      [ "$(${pkgs.coreutils}/bin/cat "$connector/status" 2>/dev/null)" = connected ] || continue
      driver=$(${pkgs.coreutils}/bin/basename "$(${pkgs.coreutils}/bin/readlink -f "$connector/device/device/driver")")
      if [ "$driver" = nvidia ]; then
        echo SUNSHINE_ENCODER=nvenc
        exit 0
      fi
    done
    echo SUNSHINE_ENCODER=vaapi
  '';

  virtualOutputScript = pkgs.writeShellScript "sunshine-virtual-output" ''
    niri=${getExe config.programs.niri.package}
    name=${cfg.virtualOutput.name}
    "$niri" msg remove-virtual-output "$name" >/dev/null 2>&1 || true
    [ "$1" = create ] || exit 0
    exec "$niri" msg create-virtual-output --name "$name" \
      --width "$SUNSHINE_CLIENT_WIDTH" \
      --height "$SUNSHINE_CLIENT_HEIGHT" \
      --refresh-rate "$SUNSHINE_CLIENT_FPS"
  '';
in
{
  options.${namespace}.services.sunshine = {
    enable = mkEnableOption "the Sunshine game stream host for Moonlight";
    user = mkOption {
      type = types.str;
      default = config.${namespace}.primaryUser;
      defaultText = "config.\${namespace}.primaryUser";
      description = "User whose graphical session is streamed.";
    };
    encoder = mkOption {
      type = types.enum [
        "auto"
        "nvenc"
        "vaapi"
      ];
      default = "vaapi";
      description = "Encoder backend Sunshine is forced to use; auto takes nvenc while an NVIDIA GPU drives the internal panel and vaapi otherwise.";
    };
    virtualOutput = {
      enable = mkEnableOption "streaming a niri virtual output created at the client's resolution for each session";
      name = mkOption {
        type = types.str;
        default = "virtual";
        description = "Name of the niri virtual output.";
      };
    };
  };

  config = mkIf cfg.enable {
    ${namespace} = {
      ports = {
        sunshine-http = basePort - 5;
        sunshine-https = basePort;
        sunshine-web = webPort;
        sunshine-video = basePort + 9;
        sunshine-control = basePort + 10;
        sunshine-audio = basePort + 11;
        sunshine-mic = basePort + 13;
        sunshine-rtsp = basePort + 21;
      };

      system.networking.firewall.public.sunshine = {
        upstream = "nixos/modules/services/networking/sunshine.nix";
        tcp = [
          (basePort - 5)
          basePort
          webPort
          (basePort + 21)
        ];
        udp = [
          (basePort + 9)
          (basePort + 10)
          (basePort + 11)
          (basePort + 13)
          (basePort + 21)
        ];
      };

      tailscale.serve."svc:${serviceName}" = {
        comment = "Sunshine game stream host web UI";
        endpoints."tcp:443" = "https+insecure://localhost:${toString webPort}";
      };
    };

    services.sunshine = {
      enable = true;
      openFirewall = true;

      settings = {
        port = basePort;
        sunshine_name = config.networking.hostName;
        # workaround for the web UI never requesting a CSRF token
        csrf_allowed_origins = concatStringsSep "," allowedOrigins;
        # workaround for kms breaking layer-shell and portal blocking on a consent dialog
        capture = "wlr";
      }
      # also picks the GPU: only vaapi skips the NVIDIA offload card
      // optionalAttrs (cfg.encoder != "auto") { inherit (cfg) encoder; }
      // optionalAttrs cfg.virtualOutput.enable {
        output_name = cfg.virtualOutput.name;
        global_prep_cmd = builtins.toJSON [
          {
            do = "${virtualOutputScript} create";
            undo = "${virtualOutputScript} remove";
          }
        ];
      };

      applications.apps = [
        {
          name = "Desktop";
          image-path = "desktop.png";
        }
      ]
      ++ optionals steam.enable [
        {
          name = "Steam Big Picture";
          image-path = "steam.png";
          detached = [ "${setsid} ${steamBin} steam://open/bigpicture" ];
          prep-cmd = [
            {
              do = "";
              undo = "${setsid} ${steamBin} steam://close/bigpicture";
            }
          ];
        }
      ];
    };

    users.users.${cfg.user}.extraGroups = [ "uinput" ];

    environment.systemPackages = [ pkgs.wtype ];

    systemd.user.services.sunshine.serviceConfig = {
      ExecStartPre = applyCredentials;
      ExecStart = mkIf (cfg.encoder == "auto") (
        mkForce "${getExe config.services.sunshine.package} ${configFile} encoder=\${SUNSHINE_ENCODER}"
      );
    };

    environment.etc."systemd/user-environment-generators/60-sunshine-encoder" = mkIf (
      cfg.encoder == "auto"
    ) { source = encoderGenerator; };

    clan.core.vars.generators.sunshine = {
      prompts.password = {
        description = "Sunshine web UI password for user '${cfg.user}'";
        type = "hidden";
        persist = true;
      };
      files.password.owner = cfg.user;
    };
  };
}
