{
  config,
  namespace,
  lib,
  ...
}:

let
  inherit (lib) mkIf mkEnableOption;
  cfg = config.${namespace}.services.pipewire;
in
{
  options.${namespace}.services.pipewire = {
    enable = mkEnableOption "Whether or not to manage pipewire.";
  };

  config = mkIf cfg.enable {
    security.rtkit.enable = true;
    services.pipewire = {
      enable = true;
      alsa.enable = true;
      alsa.support32Bit = true;
      pulse.enable = true;

      extraConfig = {
        client."99-resample"."stream.properties"."resample.quality" = 14;
        pipewire-pulse."99-resample"."stream.properties"."resample.quality" = 14;
        pipewire = {
          "92-low-latency"."context.properties" = {
            "default.clock.quantum" = 512;
            "default.clock.min-quantum" = 512;
            "default.clock.max-quantum" = 512;
          };
          "99-allowed-rates"."context.properties"."default.clock.allowed-rates" = [
            44100
            48000
            96000
            192000
          ];
        };
      };
    };
  };
}
