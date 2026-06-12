_:

_final: prev:
let
  libopenh264-cisco = prev.fetchurl {
    name = "libopenh264-2.5.1-linux64.7.so";
    url = "https://web.archive.org/web/20251005202247/http://ciscobinary.openh264.org/libopenh264-2.5.1-linux64.7.so.bz2";
    hash = "sha256-2CipRNTSu2QZWtqJzyzem8QXM7FUfQeI70n7jLIxt28=";
    downloadToTemp = true;
    nativeBuildInputs = [ prev.bzip2 ];
    postFetch = ''
      bunzip2 -c "$downloadedFile" > $out
    '';
  };

  stageOpenh264 = prev.writeShellScript "discord-stage-openh264" ''
    cache_dir="''${XDG_CONFIG_HOME:-$HOME/.config}/discord/discord_asset_cache/openh264"
    mkdir -p "$cache_dir"
    ln -sfT ${libopenh264-cisco} "$cache_dir/libopenh264-2.5.1-linux64.7.so"
  '';
in
{
  discord = prev.discord.overrideAttrs (prevAttrs: {
    postFixup = (prevAttrs.postFixup or "") + ''
      wrapProgramShell $out/opt/Discord/Discord --run "${stageOpenh264}"
    '';
  });
}
