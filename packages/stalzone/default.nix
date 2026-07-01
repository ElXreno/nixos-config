{
  lib,
  symlinkJoin,
  writeShellApplication,
  makeDesktopItem,
  copyDesktopItems,
  fetchurl,
  umu-launcher,
  proton-cachyos-x86_64_v3,
  winetricks,
  coreutils,
  findutils,
  gnugrep,
}:
let
  pname = "stalzone";
  version = "ru";

  setup = fetchurl {
    url = "https://stalzone.net/EXBO_Setup_ru.exe";
    hash = "sha256-7ovZTAebhuTENTmKnoxBNR/ILkh9IOVU0McuLvUgaLw=";
  };

  app = writeShellApplication {
    name = pname;
    runtimeInputs = [
      umu-launcher
      winetricks
      coreutils
      findutils
      gnugrep
    ];
    text = ''
            prefix="''${STALZONE_PREFIX:-''${XDG_DATA_HOME:-$HOME/.local/share}/stalzone/prefix}"
            mkdir -p "$prefix"

            WINEPREFIX="$(readlink -f "$prefix")"
            export WINEPREFIX
            export GAMEID="umu-stalzone"
            export PROTONPATH="''${STALZONE_PROTONPATH:-${proton-cachyos-x86_64_v3.steamcompattool}}"
            export PROTON_VERBS="waitforexitandrun"
            export WINEDLLOVERRIDES="''${WINEDLLOVERRIDES:-winemenubuilder.exe=d}"
            export WINEDEBUG="''${WINEDEBUG:--all}"
            export DXVK_ENABLE_NVAPI=1

            export __NV_PRIME_RENDER_OFFLOAD=1
            export __NV_PRIME_RENDER_OFFLOAD_PROVIDER=NVIDIA-G0
            export __GLX_VENDOR_LIBRARY_NAME=nvidia
            export __VK_LAYER_NV_optimus=NVIDIA_only
            export VK_DRIVER_FILES=/run/opengl-driver/share/vulkan/icd.d/nvidia_icd.json:/run/opengl-driver-32/share/vulkan/icd.d/nvidia_icd.json
            export __EGL_VENDOR_LIBRARY_FILENAMES=/run/opengl-driver/share/glvnd/egl_vendor.d/10_nvidia.json
            export PROTON_NVIDIA_LIBS_NO_32BIT="''${PROTON_NVIDIA_LIBS_NO_32BIT:-1}"
            export PROTON_DXVK_LOWLATENCY="''${PROTON_DXVK_LOWLATENCY:-1}"
            if [ -n "''${STALZONE_WAYLAND:-}" ]; then
              export PROTON_ENABLE_WAYLAND=1
            else
              export PROTON_ENABLE_WAYLAND=0
            fi
            export _JAVA_OPTIONS="''${_JAVA_OPTIONS:-} -Dprism.order=sw"

            find_launcher() {
              if [ -n "''${STALZONE_LAUNCHER_EXE:-}" ]; then
                printf '%s\n' "$STALZONE_LAUNCHER_EXE"
                return
              fi
              find "$WINEPREFIX/drive_c" \
                -iname 'ExboLauncher.exe' \
                -not -ipath '*/Temp/*' \
                2>/dev/null | head -n1
            }

            apply_fonts() {
              reg="$WINEPREFIX/user.reg"
              [ -f "$reg" ] || return 0
              cat >> "$reg" <<'STALZ_FONTS'

      [Control Panel\\Desktop]
      "FontSmoothing"="2"
      "FontSmoothingType"=dword:00000001
      "FontSmoothingGamma"=dword:00000578
      "FontSmoothingOrientation"=dword:00000001
      STALZ_FONTS
            }

            cmd="''${1:-run}"
            case "$cmd" in
              install)
                umu-run "${setup}"
                ;;
              winetricks)
                shift
                umu-run winetricks "$@"
                ;;
              prefix)
                printf '%s\n' "$WINEPREFIX"
                ;;
              exec)
                shift
                umu-run "$@"
                ;;
              fonts)
                apply_fonts && touch "$WINEPREFIX/.stalzone-fonts-v1"
                echo "stalzone: grayscale font smoothing applied (restart launcher to take effect)"
                ;;
              run)
                exe="$(find_launcher)"
                if [ -z "$exe" ]; then
                  echo "stalzone: EXBO launcher not found in prefix." >&2
                  echo "  run 'stalzone install' first, or set STALZONE_LAUNCHER_EXE=<path-in-prefix>." >&2
                  exit 1
                fi
                fontmarker="$WINEPREFIX/.stalzone-fonts-v1"
                if [ ! -e "$fontmarker" ]; then
                  apply_fonts && touch "$fontmarker"
                fi
                exec umu-run "$exe" "''${@:2}"
                ;;
              *)
                echo "usage: stalzone [run|install|winetricks <verbs>|exec <cmd>|fonts|prefix]" >&2
                exit 2
                ;;
            esac
    '';
  };

  desktopItem = makeDesktopItem {
    name = pname;
    desktopName = "STALZONE";
    comment = "Extraction-shooter MMO in the Chernobyl Exclusion Zone (EXBO launcher via Proton)";
    exec = "${pname} run";
    terminal = false;
    categories = [ "Game" ];
    keywords = [
      "stalker"
      "stalcraft"
      "exbo"
    ];
  };
in
symlinkJoin {
  name = "${pname}-${version}";
  paths = [
    app
    desktopItem
  ];
  nativeBuildInputs = [ copyDesktopItems ];
  desktopItems = [ desktopItem ];

  meta = {
    description = "STALZONE (ex-STALCRAFT: X) EXBO launcher wrapped with umu + GE-Proton";
    homepage = "https://stalzone.net/";
    license = lib.licenses.unfree;
    platforms = [ "x86_64-linux" ];
    mainProgram = pname;
  };
}
