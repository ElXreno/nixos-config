{
  config,
  namespace,
  lib,
  pkgs,
  inputs,
  ...
}:
let
  inherit (lib) mkIf mkEnableOption;
  cfg = config.${namespace}.programs.spicetify;

  spicePkgs = inputs.spicetify-nix.legacyPackages.${pkgs.stdenv.hostPlatform.system};
in
{
  options.${namespace}.programs.spicetify = {
    enable = mkEnableOption "managing Spotify through spicetify";
  };

  config = mkIf cfg.enable {
    programs.spicetify = {
      enable = true;
      wayland = true;

      enabledExtensions = with spicePkgs.extensions; [
        hidePodcasts
        history
        shuffle
        volumePercentage
      ];

      enabledCustomApps = with spicePkgs.apps; [
        lyricsPlus
        localFiles
      ];
    };

    stylix.targets.spicetify.colors.override = {
      base04 = config.lib.stylix.colors.base0B;
    };
  };
}
