{
  config,
  inputs,
  namespace,
  lib,
  pkgs,
  ...
}:

let
  inherit (lib)
    mkIf
    mkOption
    types
    concatMapStrings
    concatStringsSep
    head
    splitString
    ;
  cfg = config.${namespace}.places;

  toUri = entry: head (splitString " " entry);

  writeConfig = "${inputs.plasma-manager}/script/write_config.py";

  qtPlaces = pkgs.writeText "qt-places.json" (
    builtins.toJSON {
      "${config.xdg.configHome}/QtProject.conf".FileDialog.shortcuts = {
        value = concatStringsSep ", " (map toUri cfg);
        escapeValue = true;
        immutable = false;
        persistent = false;
        shellExpand = false;
      };
    }
  );
in
{
  options.${namespace}.places = mkOption {
    type = types.listOf types.str;
    default = [ ];
    example = [ "file:///mnt/datalake Datalake" ];
    description = ''
      Sidebar places for file managers and file dialogs, in the GTK bookmarks
      format: a URI, optionally followed by a space and a display label.

      Written to the GTK bookmarks file, read by Thunar, Nautilus, Nemo and
      every GTK and portal file chooser, and to the Qt file dialog sidebar in
      QtProject.conf. The Qt sidebar has no room for labels, so it only
      receives the URIs.
    '';
  };

  config = mkIf (cfg != [ ]) {
    assertions = [
      {
        assertion = builtins.pathExists writeConfig;
        message = "plasma-manager no longer ships script/write_config.py, ${namespace}.places needs another Qt config writer.";
      }
    ];

    xdg.configFile."gtk-3.0/bookmarks".text = concatMapStrings (entry: entry + "\n") cfg;

    home.activation.qtPlaces =
      lib.hm.dag.entryAfter
        [
          "writeBoundary"
          "configure-plasma"
        ]
        ''
          run ${pkgs.python3}/bin/python3 ${writeConfig} ${qtPlaces} "" ""
        '';
  };
}
