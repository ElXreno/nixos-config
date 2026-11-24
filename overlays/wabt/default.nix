_: _final: prev:
let
  version = "1.0.42";
in
{
  wabt =
    if prev.lib.versionAtLeast prev.wabt.version version then
      prev.wabt
    else
      prev.wabt.overrideAttrs {
        inherit version;

        src = prev.fetchFromGitHub {
          owner = "WebAssembly";
          repo = "wabt";
          tag = version;
          hash = "sha256-33XpYt42Jw2xfwBR64jVI3VYVIKD1YjND9SSI6CdkaM=";
          fetchSubmodules = true;
        };

        cmakeFlags = [
          "-DBUILD_TESTS=OFF"
          "-DCMAKE_PROJECT_VERSION=${version}"
        ];
      };
}
