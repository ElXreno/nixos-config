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
            graalvmPackages.graalvm-oracle_25-ea
            jdk24
            jre8
            zulu
          ];
        };

        headlamp = super.callPackage inputs.self.nixosModules.headlamp { };

        esp2elf = super.callPackage ../modules/esp2elf.nix { };
        bitmagnet = super.bitmagnet.overrideAttrs (
          final: prev: {
            version = "unstable-2025-08-01";

            src = super.fetchFromGitHub {
              owner = "bitmagnet-io";
              repo = "bitmagnet";
              rev = "2b9e8eadd34c037830d1fa7470b5ef2746cd6388";
              hash = "sha256-FPHBu/SdfnXICqPxEfLUsWNMwZqQ5PR6ATGFaeHuGAU=";
            };

            vendorHash = "sha256-aWFh3vytRARFEnVxTtSkvBOXZP0ke9e602BVNQ6xoRY=";

            patches = (prev.patches or [ ]) ++ [
              (super.fetchpatch {
                url = "https://github.com/bitmagnet-io/bitmagnet/pull/435/commits/8c8fdcde9a6b6f40a83870981aefee65f9521f31.patch";
                hash = "sha256-jFAsiMWjsOY0axkv7xSTrzVR66wri9fEGRhRz+5LwTs=";
              })
              (super.fetchpatch {
                url = "https://github.com/bitmagnet-io/bitmagnet/pull/435/commits/61e92b7edc6549d0c12956a02828abb62438ca1f.patch";
                hash = "sha256-nErbPtdcnCyhDrNjpGJYb73YAsF3IrVwc39EfJd2EBE=";
              })
            ];
          }
        );

        supergfxctl = super.supergfxctl.overrideAttrs (
          finalAttrs: previousAttrs: {
            postPatch = (previousAttrs.postPatch or "") + ''
              sed -i "s|/usr/bin/lsof|${super.lsof}/bin/lsof|" src/lib.rs
            '';
          }
        );

        hyprland = inputs.hyprland.packages.${super.system}.hyprland;
        xdg-desktop-portal-hyprland = inputs.hyprland.packages.${super.system}.xdg-desktop-portal-hyprland;
        split-monitor-workspaces =
          inputs.split-monitor-workspaces.packages.${super.system}.split-monitor-workspaces;
      })
    ];
  };
}
