{ fetchFromGitHub
, lib
, stdenv
, cmake
, pkg-config
, curl
, freetype
, glew
, libnotify
, miniupnpc
, openssl
, opusfile
, python3
, SDL2
, sqlite
, wavpack
, zlib
, libmysqlclient
, libogg
}:

stdenv.mkDerivation rec {
  pname = "ddnet";
  version = "15.5.4";

  src = fetchFromGitHub {
    owner = pname;
    repo = pname;
    rev = version;
    sha256 = "sha256-vJMYPaLK2CK+nbojLstXgxqIUaf7jNynpklFgtIpvGM=";
  };

  nativeBuildInputs = [ cmake pkg-config ];

  buildInputs = [
    curl
    freetype
    glew
    libnotify
    miniupnpc
    openssl
    opusfile
    python3
    SDL2
    sqlite
    wavpack
    zlib
    libmysqlclient
    libogg
  ];

  cmakeFlags = [
    "-DAUTOUPDATE=OFF" # Disable auto update
    "-DANTIBOT=ON"
    "-DUPNP=ON"
    "-DMYSQL=ON"
    "-DMYSQL_INCLUDEDIR=${libmysqlclient.dev}"
  ];

  CXXFLAGS = [ "-I${libmysqlclient.dev}/include/mariadb" ];

  patchPhase = ''sed -i "1s|^|#define DATA_DIR \"${placeholder "out"}/share/${pname}/data\"|" src/engine/shared/storage.cpp'';

  meta = with lib; {
    description = "DDraceNetwork, a cooperative racing mod of Teeworlds";
    longDescription = ''
      DDraceNetwork (DDNet) is an actively maintained version of DDRace,
      a Teeworlds modification with a unique cooperative gameplay.
      Help each other play through custom maps with up to 64 players,
      compete against the best in international tournaments, design your
      own maps, or run your own server.
    '';
    homepage = "https://ddnet.tw/";
    license = licenses.cc-by-sa-30;
    maintainers = with maintainers; [ ]; # TODO: add myself to the maintainers list
    platforms = platforms.all;
  };
}
