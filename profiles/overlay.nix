{
  config,
  inputs,
  ...
}:
{
  nixpkgs = {
    config = {
      allowUnfree = true;
      nvidia.acceptLicense = true;
      cudaSupport = config.device == "KURWA";
    };

    overlays = with inputs; [
      (_self: super: {
        deploy-rs = inputs.deploy-rs.packages.${super.system}.default;

        prismlauncher = super.prismlauncher.override {
          jdks = with super; [
            jdk17
            graalvm-ce
            jre8
            zulu
          ];
        };

        headlamp = super.callPackage inputs.self.nixosModules.headlamp { };

        esp2elf = super.callPackage ../modules/esp2elf.nix { };

        supergfxctl = super.supergfxctl.overrideAttrs (
          finalAttrs: previousAttrs: {
            postPatch = (previousAttrs.postPatch or "") + ''
              sed -i "s|/usr/bin/lsof|${super.lsof}/bin/lsof|" src/lib.rs
            '';
          }
        );

        hyprland = inputs.hyprland.packages.${super.system}.hyprland;
        xdg-desktop-portal-hyprland = inputs.hyprland.packages.${super.system}.xdg-desktop-portal-hyprland;
        split-monitor-workspaces = inputs.split-monitor-workspaces.packages.${super.system}.split-monitor-workspaces;
      })
    ];
  };
}
