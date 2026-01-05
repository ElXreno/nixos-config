{
  stdenv,
  lib,
  requireFile,
  autoPatchelfHook,
  wrapGAppsHook3,
  dpkg,
  alsa-lib,
  libgbm,
  nss,
  gtk3,
  libsForQt5,
  qt6,
  curl,
  libGL,
  vivaldi-ffmpeg-codecs,
}:

stdenv.mkDerivation rec {
  pname = "yandex-browser-corporate";
  version = "25.10.3.1047-1";

  src = requireFile {
    name = "YandexBrowser.deb";
    hash = "sha256-Q/rkShce+NdSQSlxj7vBPWa6Rd3zN8vFuD8byQ5qAso=";
    url = "https://browser.yandex.ru";
  };

  nativeBuildInputs = [
    autoPatchelfHook
    qt6.wrapQtAppsHook
    wrapGAppsHook3
    dpkg
  ];

  dontConfigure = true;
  dontBuild = true;

  buildInputs = [
    alsa-lib
    libgbm
    nss
    gtk3
    libsForQt5.libqtpas
    qt6.qtbase
    curl
  ];

  installPhase = ''
    mkdir -p $out/bin

    rm -f usr/share/applications/ru.yandex.desktop.browser.desktop
    mv usr/share/applications/{yandex-browser.desktop,${pname}.desktop}
    mv usr/share/appdata/{yandex-browser.appdata.xml,${pname}.appdata.xml}
    mv usr/share/gnome-control-center/default-apps/{yandex-browser.xml,${pname}.xml}
    mv usr/share/menu/{yandex-browser.menu,${pname}.menu}

    substituteInPlace \
        usr/share/applications/${pname}.desktop \
        usr/share/gnome-control-center/default-apps/${pname}.xml \
        usr/share/appdata/${pname}.appdata.xml \
        usr/share/menu/${pname}.menu \
        --replace-fail "Yandex Browser" "Yandex Browser Corporate" \
        --replace-warn "yandex-browser-corporate" "yandex-browser" \
        --replace-fail "yandex-browser" "yandex-browser-corporate"

    cp -r {usr/share,opt} $out/

    ln -sf $out/opt/yandex/browser/yandex-browser $out/bin/${pname}

    substituteInPlace $out/share/applications/${pname}.desktop --replace-fail /usr/ $out/
    substituteInPlace $out/share/gnome-control-center/default-apps/${pname}.xml --replace-fail /opt/yandex/browser/${pname} $out/bin/${pname}
    substituteInPlace $out/share/menu/${pname}.menu \
        --replace-fail "/opt/" "$out/opt/"

    patchelf --add-needed libGL.so.1 $out/opt/yandex/browser/libGLESv2.so
    ln -sf ${vivaldi-ffmpeg-codecs}/lib/libffmpeg.so $out/opt/yandex/browser/libffmpeg.so
  '';

  runtimeDependencies = map lib.getLib (
    [
      libGL
    ]
    ++ buildInputs
  );

  meta = with lib; {
    description = "Yandex Web Browser";
    homepage = "https://browser.yandex.ru/";
    license = licenses.unfree;
    sourceProvenance = with sourceTypes; [ binaryNativeCode ];
    maintainers = with maintainers; [
      elxreno
    ];
    platforms = [ "x86_64-linux" ];
  };
}
