{
  lib,
  stdenv,
  fetchFromGitHub,
  fetchgit,
  requireFile,
  symlinkJoin,
  writeShellApplication,
  coreboot-toolchain,
  microcode-intel,
  python3,
  perl,
  bison,
  flex,
  imagemagick,
  git,
  nasm,
  pkg-config,
  libuuid,
  openssl,
  zlib,
  ncurses,
  flashrom,
  openssh,
  coreutils,
}:
let
  blobs = requireFile {
    name = "grate-firmware-blobs";
    hashMode = "recursive";
    hash = "sha256-ubsG6J8Q5CmmkV0G/YCKRwin/O9ydIpRd3917yC32yY=";
    message = "The descriptor, ME, FSP, VBT and SPD cut from the GRATE flash dump are kept in the niks3 cache under the pin grate-firmware-blobs.";
  };

  vboot = fetchgit {
    url = "https://review.coreboot.org/vboot.git";
    rev = "f691f7675d46c8f4726146803543a095b68477c5";
    hash = "sha256-x+t+Ps0bpL+5d9n5PDveKIAL3JqEa1ZOLBu1FggVm5g=";
  };

  edk2 = fetchgit {
    url = "https://github.com/mrchromebox/edk2";
    rev = "7b08bcafb3e46df1a8164780efc3c687512eff88";
    fetchSubmodules = true;
    hash = "sha256-pV5j0SWVgacnDKRjHAIqsUtlFQrEzwyPFJWLOs0dYzE=";
  };

  ipxe = fetchgit {
    url = "https://github.com/ipxe/ipxe";
    rev = "7c39c04a537ce29dccc6f2bae9749d1d371429c1";
    hash = "sha256-tqOTv9f9teCQUgt52/cdAM9kE5678Bk9956cWDTDZAg=";
  };

  toolchain = symlinkJoin {
    name = "coreboot-toolchain";
    paths = [
      coreboot-toolchain.i386
      coreboot-toolchain.x64
    ];
  };

  grate-firmware = stdenv.mkDerivation {
    pname = "grate-firmware";
    version = "0-unstable-2026-11-25";

    src = fetchFromGitHub {
      owner = "ElXreno";
      repo = "coreboot";
      rev = "d85aa0d153161041aba38a204cf03f881160cb8e";
      hash = "sha256-Uj3Nbzu+GoHG3f5DDtSaRS8efJl1WaeM4lQDV4lIXIw=";
    };

    nativeBuildInputs = [
      python3
      perl
      bison
      flex
      imagemagick
      git
      nasm
      pkg-config
    ];

    buildInputs = [
      libuuid
      openssl
      zlib
      ncurses
    ];

    hardeningDisable = [ "all" ];
    enableParallelBuilding = true;

    env = {
      XGCCPATH = "${toolchain}/bin/";
      BUILD_TIMELESS = "1";
    };

    postPatch = ''
      patchShebangs util

      rm -rf 3rdparty/vboot
      cp -r ${vboot} 3rdparty/vboot
      chmod -R u+w 3rdparty/vboot

      export HOME=$TMPDIR
      repo() {
        mkdir -p "$(dirname "$2")"
        cp -r "$1" "$2"
        chmod -R u+w "$2"
        patchShebangs "$2" > /dev/null
        git -C "$2" init -q -b master
        git -C "$2" add -A
        git -C "$2" -c user.name=nix -c user.email=nix@localhost commit -q -m source
      }

      repo ${edk2} payloads/external/edk2/workspace/mrchromebox
      git -C payloads/external/edk2/workspace/mrchromebox update-ref refs/remotes/origin/uefipayload_2608 HEAD

      repo ${ipxe} payloads/external/iPXE/ipxe
      git -C payloads/external/iPXE/ipxe tag coreboot-stable
      git -C payloads/external/iPXE/ipxe tag v1.21.1
      substituteInPlace payloads/external/iPXE/Makefile \
        --replace-fail 'STABLE_COMMIT_ID=7c39c04a537ce29dccc6f2bae9749d1d371429c1' 'STABLE_COMMIT_ID=coreboot-stable'

      mkdir -p src/mainboard/morshow/mx_jb01/spd
      for i in 0 1; do
        od -An -tx1 -v -w16 ${blobs}/spd-$i.bin | sed 's/^ //' > src/mainboard/morshow/mx_jb01/spd/vendor-$i.spd.hex
      done

      sed -e 's|@BLOBS@|${blobs}|g' \
        -e 's|@MICROCODE@|${microcode-intel.src}/intel-ucode/06-9c-00|g' \
        ${./defconfig} > grate.defconfig
    '';

    configurePhase = ''
      runHook preConfigure
      make DOTCONFIG=.config KBUILD_DEFCONFIG=grate.defconfig defconfig
      runHook postConfigure
    '';

    buildPhase = ''
      runHook preBuild
      make -j$NIX_BUILD_CORES
      runHook postBuild
    '';

    installPhase = ''
      runHook preInstall
      install -Dm644 build/coreboot.rom $out/coreboot.rom
      install -Dm644 .config $out/config
      runHook postInstall
    '';

    meta = {
      description = "coreboot with edk2 for GRATE (Morshow MX-JB01)";
      license = lib.licenses.unfree;
      platforms = [ "x86_64-linux" ];
    };
  };
in
{
  inherit grate-firmware;

  grate-firmware-flash = writeShellApplication {
    name = "grate-firmware-flash";
    excludeShellChecks = [ "SC2029" ];
    runtimeInputs = [
      openssh
      coreutils
    ];
    text = ''
      host=''${1:-grate}
      image=${grate-firmware}/coreboot.rom
      flashrom=${flashrom}/bin/flashrom

      echo "image $image"
      sha256sum "$image"
      nix copy --to "ssh-ng://root@$host" ${flashrom}
      backup=/var/tmp/grate-flash-before-$(date +%Y%m%d-%H%M%S).bin
      ssh "root@$host" "cat /dev/mtd0ro > $backup && sha256sum $backup"
      echo "current flash saved on $host as $backup"

      read -r -p "Write the COREBOOT and CONSOLE regions of this image to $host? Type yes: " answer
      [[ $answer == yes ]] || { echo "not written"; exit 1; }

      scp "$image" "root@$host:/var/tmp/grate-coreboot.rom"
      ssh "root@$host" "bash -c 'spi=/sys/bus/pci/drivers/intel-spi; dev=0000:00:1f.5
        echo \$dev > \$spi/unbind
        $flashrom -p internal --fmap -i COREBOOT -i CONSOLE -w /var/tmp/grate-coreboot.rom; status=\$?
        echo \$dev > \$spi/bind
        exit \$status'"
    '';
  };
}
