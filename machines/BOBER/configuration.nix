{
  inputs,
  namespace,
  pkgs,
  ...
}:
{
  imports = [ inputs.courbet.nixosModules.default ];

  ${namespace} = {
    desktop-environments.plasma-mobile.enable = true;
    system.boot.kernel.packages = pkgs.linuxPackagesFor inputs.courbet.packages.x86_64-linux.kernel;
    services.monitoring.agent.journal.enable = false;
  };

  hardware.courbet.chargeLimit.enable = true;

  hardware.courbet.zapShader = pkgs.requireFile {
    name = "a615_zap.mbn";
    hash = "sha256-j2xZMBjyy7QT9I/B2v/YhScJ4cTNek3bDdLk1DlxwbM=";
    message = "a615_zap.mbn comes from the courbet vendor partition and is kept in the niks3 cache under the pin bober-zap-shader.";
  };

  hardware.courbet.ipaFirmware = pkgs.requireFile {
    name = "ipa_fws.mbn";
    hash = "sha256-8SmJ9+ESXfPe3EpQ3rUmosj+L32WZgqD7PHozwqS1Lk=";
    message = "ipa_fws.mbn comes from the courbet vendor partition and is kept in the niks3 cache under the pin bober-ipa-firmware.";
  };
}
