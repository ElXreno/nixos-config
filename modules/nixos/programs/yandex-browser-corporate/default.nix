{
  config,
  namespace,
  lib,
  pkgs,
  ...
}:

let
  inherit (lib)
    mkIf
    mkEnableOption
    mkOption
    types
    optionalString
    ;
  cfg = config.${namespace}.programs.yandex-browser-corporate;
in
{
  options.${namespace}.programs.yandex-browser-corporate = {
    enable = mkEnableOption "Whether or not to manage Yandex Browser Corporate.";
    licenseFilePath = mkOption {
      type = with types; nullOr str;
      default = null;
      description = "Path to the license file. Will be symlinked to /var/lib/yandex/browser-license/license.";
    };
  };

  config = mkIf cfg.enable {
    environment.systemPackages = with pkgs.${namespace}; [
      yandex-browser-corporate
    ];

    systemd.services."setup-yandex-browser-corporate" = {
      wantedBy = [ "sysinit.target" ];
      script =
        let
          baseDir = "/var/lib/yandex";
          browser = "${pkgs.${namespace}.yandex-browser-corporate}";
        in
        ''
          rm -rf ${baseDir} || true
          mkdir -p ${baseDir}/browser/{,resources/configs,Extensions,resources/wallpapers}

          cp ${browser}/opt/yandex/browser/{partner_config,master_preferences} ${baseDir}/browser/
          cp ${browser}/opt/yandex/browser/resources/configs/all_zip ${baseDir}/browser/resources/configs/
          cp -r ${browser}/opt/yandex/browser/Extensions/* ${baseDir}/browser/Extensions/
          cp -r ${browser}/opt/yandex/browser/resources/{tablo*,*.png,wallpapers} ${baseDir}/browser/resources/

          cp -r ${pkgs.${namespace}.yandex-browser-customisation}${baseDir}/* ${baseDir}

          ${optionalString (cfg.licenseFilePath != null) ''
            mkdir -p ${baseDir}/browser-license
            ln -sn ${cfg.licenseFilePath} ${baseDir}/browser-license/license
          ''}
        '';
      serviceConfig = {
        Type = "oneshot";
        User = "root";
      };
    };
  };
}
