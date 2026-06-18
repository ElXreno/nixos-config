{
  pkgs,
  fetchurl,
}:
let
  modpack = pkgs.fetchCurseForgeModpack {
    pname = "ATM10-To-the-Sky";
    version = "2.0.2";
    src = fetchurl {
      name = "ATM10-To-the-Sky-2.0.2.zip";
      url = "https://mediafilez.forgecdn.net/files/7854/204/ATM10%20To%20the%20Sky-2.0.2.zip";
      sha256 = "sha256-4/YPJMIVWyWh7PB8DV1k/SXnsyC2BCoHtgMYMdpYrYA=";
    };
    locks = ./locks.json;
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
    url = "https://cdn.modrinth.com/data/COlSi5iR/versions/KmfiVd28/c2me-neoforge-mc1.21.1-0.3.0%2Balpha.0.93.jar";
    sha256 = "sha256-JzWxbhNuUcA8moIR++yvnVcaKEdZgSI8YGYkZWZPUyI=";
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
