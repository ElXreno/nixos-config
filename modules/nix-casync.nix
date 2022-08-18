{ buildGoModule
, fetchFromGitHub
, lib
}:

buildGoModule rec {
  pname = "nix-casync";
  version = "0.5.1";

  src = fetchFromGitHub {
    owner = "flokli";
    repo = pname;
    rev = "7a81a6b8f088b83c348e6cf715743e80f29550a9";
    sha256 = "sha256-+YuQ9jhmIYxEUjV4hEtLHExOsNDWE63NnTlUaGRQ4Sg=";
  };

  vendorSha256 = "sha256-jp1geLRmNOB6Mo7DywwYZLHaF3cZbsO6KiCBFPNVhVA=";

  proxyVendor = true;

  meta = with lib; {
    description = "A more efficient way to store and substitute Nix store paths";
    longDescription = ''
      A more efficient way to store and substitute Nix store paths.
    '';
    homepage = "https://github.com/flokli/nix-casync";
    license = licenses.asl20;
    maintainers = with maintainers; [ ]; # TODO: add myself to the maintainers list
    platforms = platforms.all;
  };
}
