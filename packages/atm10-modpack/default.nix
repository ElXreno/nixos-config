{
  pkgs,
  fetchurl,
}:
let
  modpack = pkgs.fetchCurseForgeModpack {
    pname = "All-the-Mods-10";
    version = "6.6";
    src = fetchurl {
      name = "All-the-Mods-10-6.6.zip";
      url = "https://edge.forgecdn.net/files/7892/974/All%20the%20Mods%2010-6.6.zip";
      sha256 = "sha256-G+pEftXVDYWnxjLqTaEf9k8esiItUl/tCmtBnqZY5e8=";
    };
    locks = ./locks.json;
    extraExcludes = [ "relics-mod" ];
  };
in
modpack.addFiles {
  "mods/packetfixer-3.3.1.jar" = fetchurl {
    url = "https://cdn.modrinth.com/data/c7m1mi73/versions/2C41Q8WX/packetfixer-3.3.1-1.20.5-1.21.X-merged.jar";
    sha256 = "sha256-KXmoo/ROXV30+fYxXRjCIPPev3NruKLCdArN+xO9gg8=";
  };
  "mods/bluemap-5.7-neoforge.jar" = fetchurl {
    url = "https://cdn.modrinth.com/data/swbUV1cr/versions/8iJcPOHJ/bluemap-5.7-neoforge.jar";
    sha256 = "sha256-gnrDkhmG0NXpwk7yKA9HwRDSLcFNKE77ZTMAENLpUX0=";
  };
  "mods/c2me-neoforge-0.3.0.jar" = fetchurl {
    url = "https://cdn.modrinth.com/data/COlSi5iR/versions/9iPiN34N/c2me-neoforge-mc1.21.1-0.3.0%2Balpha.0.91.jar";
    sha256 = "sha256-EH7Fs8ygWgOO4hqjXlxoiAKXw3DkpH0UmY7PpycK82k=";
  };
  "mods/relics-1.21.1-0.11.16.jar" = fetchurl {
    url = "https://cdn.modrinth.com/data/OCJRPujW/versions/m7HHr31k/relics-1.21.1-0.11.16.jar";
    sha256 = "sha256-YoTdJMrM0QQdBP939gpkg55awI8VrwHW4tBto0DKoHI=";
  };
  "mods/lithium-neoforge-0.15.3.jar" = fetchurl {
    url = "https://cdn.modrinth.com/data/gvQqBUqZ/versions/RXHf27Wv/lithium-neoforge-0.15.3%2Bmc1.21.1.jar";
    sha256 = "sha256-plSIYr0T/47qZ0fPYd9QyDdKFqItBnYU5BTtYA7ubYM=";
  };
  "mods/scalablelux-0.1.0.1-neoforge.jar" = fetchurl {
    url = "https://cdn.modrinth.com/data/Ps1zyz6x/versions/j10HNoNf/ScalableLux-0.1.0.1%2Bneoforge.1cb1e91-all.jar";
    sha256 = "sha256-dDQKN6+FgBTs9mBoBUvtOrd4si0klW00ii7CvDxYXLU=";
  };
  "mods/saturn-0.1.5.jar" = fetchurl {
    url = "https://cdn.modrinth.com/data/2eT495vq/versions/V5lIEjKs/saturn-mc1.21.1-0.1.5.jar";
    sha256 = "sha256-9J9LUfrrTp63Vocj+iWcDaHDRg+AGVl/BKVMITe3bDI=";
  };
}
