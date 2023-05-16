{ lib, stdenvNoCC, fetchFromGitHub }:
with lib;
stdenvNoCC.mkDerivation {
  pname = "rtl8723b-firmware";
  version = "2015-01-30";
  src = fetchFromGitHub {
    owner = "lwfinger";
    repo = "rtl8723au_bt";
    rev = "3c7f2f2274731ade88d609929fa5f5a578a18b9c";
    sha256 = "sha256-dKqeJ+mj/EsS2KXMpdhpOJK+W48kR038ZE3ugoppeY8=";
  };

  dontBuild = true;

  basePath = "Linux_BT_USB_2.11.20140423_8723BE/8723B";
  installPhase = ''
    mkdir -p "$out/lib/firmware/rtl_bt"
    cp $basePath/rtl8723b_config "$out/lib/firmware/rtl_bt/rtl8723b_config.bin"
  '';

  meta = with lib; {
    description = "Firmware for RealTek 8723b";
    homepage = "https://github.com/lwfinger/rtl8723au_bt";
    license = licenses.unfreeRedistributableFirmware;
    maintainers = with maintainers; [ elxreno ];
    platforms = with platforms; linux;
  };
}
