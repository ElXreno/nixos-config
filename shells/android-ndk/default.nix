{ pkgs, ... }:
let
  ndkVersion = "27.3.13750724";

  android = (pkgs.androidenv.override { licenseAccepted = true; }).composeAndroidPackages {
    includeNDK = true;
    ndkVersions = [ ndkVersion ];
  };

  ndkHome = "${android.androidsdk}/libexec/android-sdk/ndk/${ndkVersion}";
in
pkgs.mkShell {
  packages = [
    android.androidsdk
    pkgs.cmake
    pkgs.ninja
  ];

  env = {
    ANDROID_HOME = "${android.androidsdk}/libexec/android-sdk";
    ANDROID_NDK_HOME = ndkHome;
    ANDROID_NDK_ROOT = ndkHome;
  };
}
