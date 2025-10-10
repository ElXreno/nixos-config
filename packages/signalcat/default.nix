{
  python3Packages,
  fetchPypi,
  lib,
}:
python3Packages.buildPythonPackage rec {
  pname = "signalcat";
  version = "1.4.0";
  format = "pyproject";

  src = fetchPypi {
    inherit pname version;
    hash = "sha256-MD5NRah01rls4Tc00pkVreeq7NothQjOzbMWLgWPolg=";
  };

  propagatedBuildInputs = with python3Packages; [
    hatchling

    pyserial
    pyusb
    bitstring
    packaging
  ];

  meta = with lib; {
    description = "Signaling Collection and Analysis Tool";
    homepage = "https://github.com/fgsect/scat";
    mainProgram = "scat";
    license = licenses.gpl2;
    platforms = platforms.linux;
  };
}
