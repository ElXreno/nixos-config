{
  lib,
  stdenv,
  buildHomeAssistantComponent,
  fetchFromGitHub,
  fetchurl,
  autoPatchelfHook,
  home-assistant,
}:
let
  mini-racer = home-assistant.python.pkgs.buildPythonPackage rec {
    pname = "mini-racer";
    version = "0.14.1";
    format = "wheel";

    src =
      let
        wheels = {
          x86_64-linux = {
            platform = "manylinux_2_27_x86_64";
            hash = "sha256-zfOgiOE2PxamlSiPiCq/drNwW44d8hQYIIuH7QEAN6Q=";
          };
          aarch64-linux = {
            platform = "manylinux_2_27_aarch64";
            hash = "sha256-f5PZGXPdstpOiZ4G7LQmv+fqEDzRySaH7Jw48gbzdes=";
          };
        };
        wheel = wheels.${stdenv.hostPlatform.system};
      in
      fetchurl {
        url = "https://files.pythonhosted.org/packages/py3/m/mini-racer/mini_racer-${version}-py3-none-${wheel.platform}.whl";
        inherit (wheel) hash;
      };

    nativeBuildInputs = [ autoPatchelfHook ];
    buildInputs = [ stdenv.cc.cc.lib ];

    dontStrip = true;
    pythonImportsCheck = [ "py_mini_racer" ];

    meta = {
      description = "Minimal, modern embedded V8 for Python";
      homepage = "https://github.com/bpcreech/PyMiniRacer";
      license = lib.licenses.isc;
      platforms = lib.attrNames {
        x86_64-linux = null;
        aarch64-linux = null;
      };
      sourceProvenance = with lib.sourceTypes; [ binaryNativeCode ];
    };
  };
in
buildHomeAssistantComponent rec {
  owner = "Tasshack";
  domain = "dreame_vacuum";
  version = "2.0.0b22";

  src = fetchFromGitHub {
    inherit owner;
    repo = "dreame-vacuum";
    tag = "v${version}";
    hash = "sha256-Ze7Pn3fDbEjUMzeNJ3MZ+rYPIjQG74/Kpf8lPwyd0RU=";
  };

  dependencies = [
    mini-racer
  ]
  ++ (with home-assistant.python.pkgs; [
    pillow
    numpy
    pybase64
    requests
    pycryptodome
    python-miio
    paho-mqtt
  ]);

  dontCheckManifest = true;

  meta = {
    changelog = "https://github.com/${owner}/dreame-vacuum/releases/tag/${src.tag}";
    description = "Home Assistant integration for Dreame robot vacuums with map support";
    homepage = "https://github.com/${owner}/dreame-vacuum";
    license = lib.licenses.mit;
    maintainers = with lib.maintainers; [ elxreno ];
  };
}
