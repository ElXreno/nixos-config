lib:
let
  inherit (lib) mkOption types;
in
{
  path = mkOption {
    type = types.str;
    description = "Absolute path to snapshot, or the name a `command` source is filed under.";
  };
  command = mkOption {
    type = types.nullOr types.str;
    default = null;
    example = "pg_dump mydb";
    description = "Shell pipeline whose stdout becomes the source, instead of reading `path` from disk.";
  };
  fileName = mkOption {
    type = types.nullOr types.str;
    default = null;
    description = "Name the streamed content is stored under, the basename of `path` when null.";
  };
  preCommand = mkOption {
    type = types.lines;
    default = "";
    description = "Shell run before this source is snapshotted, to quiesce whatever writes to it.";
  };
  postCommand = mkOption {
    type = types.lines;
    default = "";
    description = ''
      Shell run after this source is snapshotted. It also runs when the snapshot fails or the
      unit is stopped, so it must be safe to run twice.
    '';
  };
  ignore = mkOption {
    type = types.listOf types.str;
    default = [ ];
    description = "Ignore rules anchored at the source root.";
  };
  compression = mkOption {
    type = types.nullOr types.str;
    default = null;
    description = "Compressor override for this source.";
  };
  policy = mkOption {
    type = types.attrsOf types.anything;
    default = { };
    description = "Extra fields merged into this source's policy.";
  };
}
