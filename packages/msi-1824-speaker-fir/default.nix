{
  lib,
  stdenvNoCC,
  fetchurl,
  unzip,
  cabextract,
  python3,
}:

stdenvNoCC.mkDerivation (finalAttrs: {
  pname = "msi-1824-speaker-fir";
  version = "4.15.4.0";

  src = fetchurl {
    url = "https://download-2.msi.com/nb_drivers/ad/MSI-NahimicRestoreTool-APO4-SWCv4.15.4.0-EXTv1.7.3.0_${finalAttrs.version}_0x28fe1840.zip";
    hash = "sha256-eNs/L8dBzz2T98wFWPjSyewnjK2lcKtutfba+d1hIp0=";
  };

  nativeBuildInputs = [
    unzip
    cabextract
    python3
  ];

  unpackPhase = ''
    runHook preUnpack
    unzip -q $src -d outer
    unzip -q -j outer/Nahimic/*.exe '*NH3ProductSettings3.cab' -d inner || [ -f inner/NH3ProductSettings3.cab ]
    cabextract -q -d settings -F 'Devices/14621500_InternalSpeakers.nsx' inner/NH3ProductSettings3.cab
    runHook postUnpack
  '';

  installPhase = ''
    runHook preInstall
    mkdir -p $out/share/msi-1824
    python3 ${./nsx-to-wav.py} settings/Devices/14621500_InternalSpeakers.nsx $out/share/msi-1824/speakers-fir.wav
    runHook postInstall
  '';

  meta = {
    description = "Speaker correction FIR for the MSI Titan 18 HX A2W, extracted from MSI's Nahimic device settings";
    license = lib.licenses.unfree;
    platforms = lib.platforms.all;
  };
})
