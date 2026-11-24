{
  rea,
  ghidra,
  openjdk25,
  symlinkJoin,
  makeWrapper,
}:
symlinkJoin {
  name = "rea-with-ghidra-${rea.version}";
  paths = [ rea ];
  nativeBuildInputs = [ makeWrapper ];
  postBuild = ''
    for bin in $out/bin/*; do
      wrapProgram "$bin" \
        --set-default GHIDRA_INSTALL_DIR ${ghidra}/lib/ghidra \
        --set-default JAVA_HOME ${openjdk25.home}
    done
  '';
  passthru.skill = "${rea}/lib/node_modules/rea-agents/skills/reverse-engineer-anything";
  meta = rea.meta // {
    description = "REA with the Ghidra provider preconfigured";
  };
}
