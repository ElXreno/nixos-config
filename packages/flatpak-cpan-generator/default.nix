{
  buildFHSEnv,
  fetchFromGitHub,
  perl,
  openssl,
  gnumake,
  pkg-config,
  gcc,
  libxcrypt,
}:
let
  customPerl = perl.withPackages (
    p: with p; [
      CaptureTiny
      GetoptLongDescriptive
      JSONMaybeXS
      LWP
      LWPProtocolHttps
      MetaCPANClient
      Appcpanminus
    ]
  );

  src = fetchFromGitHub {
    owner = "flatpak";
    repo = "flatpak-builder-tools";
    rev = "fdbe66a48b7450f3c18ab2cb8ff31be704846600";
    hash = "sha256-LpzAIylwDi/R0CO21JHMNjSG9MBMK11h8TIW/eL5UaU=";
  };
in
buildFHSEnv {
  name = "flatpak-cpan-generator";

  targetPkgs = _pkgs: [
    customPerl
    openssl
    openssl.dev
    gnumake
    libxcrypt
    pkg-config
    gcc
  ];

  runScript = "${src}/cpan/flatpak-cpan-generator.pl";
}
