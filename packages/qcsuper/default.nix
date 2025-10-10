{
  python3Packages,
  fetchPypi,
  lib,
}:
let
  pycrate = python3Packages.buildPythonPackage rec {
    pname = "pycrate";
    version = "0.7.11";
    format = "setuptools";

    src = fetchPypi {
      inherit pname version;
      hash = "sha256-I9wcGbFIoXY3/qcxZrgBx2euc4zEyz2LH3SinkadfAI=";
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
  version = "2.0.1";
  format = "setuptools";

  src = fetchPypi {
    inherit pname version;
    hash = "sha256-4L6MZpqhtfTUj20A5cBSW+AqzbW3rIUtlZIEdDgjdjw=";
  };

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
