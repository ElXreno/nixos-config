{
  python3Packages,
  fetchPypi,
  lib,
}:
let
  pycrate = python3Packages.buildPythonPackage rec {
    pname = "pycrate";
    version = "0.8.1";
    format = "setuptools";

    src = fetchPypi {
      inherit pname version;
      hash = "sha256-pAjJQtIZxkalqBBzuANvSKgcRUDdHyX9ovi73gYH25g=";
    };

    meta = with lib; {
      description = "A Python library to ease the development of encoders and decoders for various protocols and file formats, especially telecom ones. Provides an ASN.1 compiler and a CSN.1 runtime.";
      homepage = "https://github.com/pycrate-org/pycrate/";
      mainProgram = pname;
      license = licenses.lgpl21;
      platforms = platforms.linux;
    };
  };
in
python3Packages.buildPythonPackage rec {
  pname = "qcsuper";
  version = "2.1.1";
  format = "pyproject";

  src = fetchPypi {
    inherit pname version;
    hash = "sha256-J8rfEqJosVAJwcluzTPU6Bbs2RrtG4LvZSZ16rLfh1A=";
  };

  nativeBuildInputs = with python3Packages; [
    poetry-core
  ];

  propagatedBuildInputs = with python3Packages; [
    pyserial
    pyusb
    crcmod
    pycrate
  ];

  meta = with lib; {
    description = "QCSuper is a tool communicating with Qualcomm-based phones and modems, allowing to capture raw 2G/3G/4G radio frames, among other things.";
    homepage = "https://github.com/P1sec/QCSuper";
    mainProgram = pname;
    license = licenses.gpl3;
    platforms = platforms.linux;
  };
}
