{ lib, rust, stdenv, rustPlatform, fetchFromGitHub }:

rustPlatform.buildRustPackage rec {
  pname = "elfshaker";
  version = "0.9.1";

  src = fetchFromGitHub {
    owner = pname;
    repo = pname;
    rev = "43dc07688990235271210b8fbaea1f27efbb853b";
    sha256 = "sha256-LZVK7UXEuWmTivWdHDF7PZBKYZVo0AP6yRISnVG8bbY=";
  };

  cargoSha256 = "sha256-WizEWb1wYvYFbIZAT3WEXmirvs2LB64Qbm4zv48J+c0=";

  meta = with lib; {
    description = "elfshaker stores binary objects efficiently";
    longDescription = ''
      elfshaker is a low-footprint, high-performance version control system
      fine-tuned for binaries.
    '';
    homepage = "https://github.com/elfshaker/elfshaker";
    # changelog = "https://github.com/elfshaker/elfshaker/releases/tag/v${version}";
    license = licenses.asl20;
    maintainers = [ ];
  };
}
