{
  lib,
  namespace,
  rea,
  ghidra,
  openjdk25,
  binutils-unwrapped-all-targets,
  rizin,
  gdb,
  wireshark-cli,
  binwalk,
  unblob,
  apktool,
  android-tools,
  ilspycmd,
  util-linux,
  dconf,
  xprop,
  wabt,
  pkgs,
  symlinkJoin,
  makeWrapper,
}:
let
  inherit (pkgs.${namespace}) jadx-headless-mcp wakaru;
  prlimit = lib.getExe' util-linux "prlimit";

  env = {
    GHIDRA_INSTALL_DIR = "${ghidra}/lib/ghidra";
    JAVA_HOME = openjdk25.home;
    REA_OBJDUMP_COMMAND = lib.getExe' binutils-unwrapped-all-targets "objdump";
    REA_RIZIN_COMMAND = lib.getExe' rizin "rizin";
    REA_GDB_COMMAND = lib.getExe' gdb "gdb";
    REA_TSHARK_COMMAND = lib.getExe' wireshark-cli "tshark";
    REA_CAPINFOS_COMMAND = lib.getExe' wireshark-cli "capinfos";
    REA_BINWALK_COMMAND = lib.getExe binwalk;
    REA_UNBLOB_COMMAND = lib.getExe unblob;
    REA_APKTOOL_COMMAND = lib.getExe apktool;
    REA_ADB_PATH = lib.getExe' android-tools "adb";
    REA_ILSPY_CMD_PATH = lib.getExe ilspycmd;
    REA_DCONF_COMMAND = lib.getExe dconf;
    REA_XPROP_COMMAND = lib.getExe xprop;
    REA_WABT_BIN_DIRECTORY = "${wabt}/bin";
    REA_JADX_MCP_JAR = jadx-headless-mcp.jar;
    REA_WAKARU_COMMAND = lib.getExe wakaru;
    REA_TSHARK_PRLIMIT_COMMAND = prlimit;
    REA_SIGROK_PRLIMIT_COMMAND = prlimit;
    REA_FIRMWARE_PRLIMIT_COMMAND = prlimit;
    REA_EVM_PRLIMIT_COMMAND = prlimit;
    REA_JAVASCRIPT_PRLIMIT_COMMAND = prlimit;
  };
in
symlinkJoin {
  name = "rea-full-${rea.version}";
  paths = [
    rea
    ilspycmd
  ];
  nativeBuildInputs = [ makeWrapper ];
  postBuild = ''
    for bin in $out/bin/rea $out/bin/rea-agents; do
      wrapProgram "$bin" ${
        lib.concatStringsSep " " (
          lib.mapAttrsToList (name: value: "--set-default ${name} ${lib.escapeShellArg value}") env
        )
      }
    done
  '';
  passthru.skill = "${rea}/lib/node_modules/rea-agents/skills/reverse-engineer-anything";
  meta = rea.meta // {
    description = "REA with its external analysis tools preconfigured";
  };
}
