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

        cfipv4 =
          let
            fileToList = x: lib.strings.splitString "\n" (builtins.readFile x);
          in
          fileToList (
            super.fetchurl {
              url = "https://www.cloudflare.com/ips-v4";
              hash = "sha256-8Cxtg7wBqwroV3Fg4DbXAMdFU1m84FTfiE5dfZ5Onns=";
            }
          );
      })
    ];
  };
}
