{
  lib,
  stdenv,
  fetchgit,
}:

stdenv.mkDerivation {
  pname = "schbench";
  version = "0-unstable-2026-05-08";

  src = fetchgit {
    url = "https://git.kernel.org/pub/scm/linux/kernel/git/mason/schbench.git";
    rev = "6300b8f3a8922c61ea6bb2cdfa1901a42c0cc6fc";
    hash = "sha256-2GmVrZ9YNg3TpifxMaHlBS0bmDBKGQhqBXUGpFCz/jo=";
  };

  enableParallelBuilding = true;

  installPhase = "install -Dm755 schbench $out/bin/schbench";

  meta = {
    description = "Scheduler benchmark for measuring wakeup and request latencies";
    homepage = "https://git.kernel.org/pub/scm/linux/kernel/git/mason/schbench.git/";
    license = lib.licenses.gpl2Only;
    platforms = lib.platforms.linux;
    mainProgram = "schbench";
  };
}
