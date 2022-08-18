{ fetchFromGitHub
, lib
, stdenv
, python3Packages
}:

# FIX PACKAGE

stdenv.mkDerivation rec {
  pname = "cassowary";
  version = "0.4";

  src = fetchFromGitHub {
    owner = "casualsnek";
    repo = pname;
    rev = "v${version}";
    sha256 = "sha256-bU2dWHl6CiyQAwhBF3ESLcdyzzn8q7VtFvgglj/GmGg=";
  };

  nativeBuildInputs = with python3Packages; [ python build ];

  propagatedBuildInputs = with python3Packages; [ setuptools wheel ];

  patchPhase = ''
    patchShebangs app-linux/build.sh
  '';

  buildPhase = ''
    cd app-linux
    python3 -m build --no-isolation
    tar xzf dist/cassowary-0.4.tar.gz -C dist
  '';

  installPhase = ''
    ls dist/${pname}-${version}
    mv dist/${pname}-${version} $out
  '';

  meta = with lib; {
    description = "Run Windows Applications on Linux as if they are native";
    longDescription = ''
      Run Windows Applications on Linux as if they are native,
      Use linux applications to launch files files located in
      windows vm without needing to install applications on vm.
      With easy to use configuration GUI.
    '';
    homepage = "https://github.com/casualsnek/cassowary";
    license = licenses.gpl2Only;
    maintainers = with maintainers; [ elxreno ];
    platforms = platforms.linux;
  };
}
