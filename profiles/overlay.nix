{
  config,
  inputs,
  lib,
  ...
}:
let
  optimizedPkgsZnver4 = import inputs.nixpkgs {
    localSystem = {
      inherit (config.nixpkgs) system;
      gcc.arch = "znver4";
      gcc.tune = "znver4";
    };
  };
in
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
          jdks =
            with super;
            let
              myjre8 =
                if (lib.elem "gccarch-znver4" config.nix.settings.system-features) then
                  optimizedPkgsZnver4.jre8
                else
                  jre8;
            in
            [
              jdk17
              graalvm-ce
              myjre8
            ];
        };

        headlamp = super.callPackage inputs.self.nixosModules.headlamp { };

        esp2elf = super.callPackage ../modules/esp2elf.nix { };

        supergfxctl = super.supergfxctl.overrideAttrs (finalAttrs: previousAttrs: {
          postPatch = (previousAttrs.postPatch or "") + ''
            sed -i "s|/usr/bin/lsof|${super.lsof}/bin/lsof|" src/lib.rs
          '';
        });
      })
    ];
  };
}
