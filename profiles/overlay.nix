{ config, inputs, pkgs, lib, ... }:
{
  nixpkgs = {
    config = {
      allowUnfreePredicate = pkg: builtins.elem (lib.getName pkg) [
        "gitkraken"
        "graalvm17-ee"
        "megasync"
        "ngrok"
        "nvidia-settings"
        "nvidia-x11"
        "steam-original"
        "steam-run"
        "steam"
        "unrar"
        "vscode"

        # IDE
        "android-studio-canary"
        "clion"
        "idea-ultimate"
        "phpstorm"
        "pycharm-professional"
        "rider"

        "vscode-extension-github-copilot"
      ];
    };
    overlays = with inputs; [
      rust-overlay.overlays.default
      (_self: super:
        {
          bluez5-experimental = super.bluez5-experimental.overrideAttrs (old: {
            patches = (old.patches or [ ]) == [
              (super.fetchpatch {
                url = "https://patchwork.kernel.org/project/bluetooth/patch/20210514211304.17237-1-luiz.dentz@gmail.com/raw/";
                sha256 = "sha256-SnERSCMo7KPgZV4yC1eYwDBg+iPxoB0Ve7l2VX97KrA=";
              })
            ];
          });

          linuxPackages_xanmod_latest = super.linuxPackagesFor (super.linuxPackages_xanmod_latest.kernel.override {
            kernelPatches = [
              {
                name = "amd-pstate-epp-v12_1";
                patch = super.fetchpatch {
                  url = "https://lore.kernel.org/lkml/20230131090016.3970625-2-perry.yuan@amd.com/raw";
                  sha256 = "sha256-X1oJAI9Le0SYnr+00b74dBZJ+7bg50nJgkXlcIhv7k0=";
                };
              }
              {
                name = "amd-pstate-epp-v12_2";
                patch = super.fetchpatch {
                  url = "https://lore.kernel.org/lkml/20230131090016.3970625-3-perry.yuan@amd.com/raw";
                  sha256 = "sha256-ccqfDbwamTROCMC5QOPEmrAHSYSoyXMLx8V9zmVhDyc=";
                };
              }
              {
                name = "amd-pstate-epp-v12_3";
                patch = super.fetchpatch {
                  url = "https://lore.kernel.org/lkml/20230131090016.3970625-4-perry.yuan@amd.com/raw";
                  sha256 = "sha256-KXwmopM3WX46nDvWvlTMXI4cDw9KsHeUUq9iytHWbks=";
                };
              }
              {
                name = "amd-pstate-epp-v12_4";
                patch = super.fetchpatch {
                  url = "https://lore.kernel.org/lkml/20230131090016.3970625-5-perry.yuan@amd.com/raw";
                  sha256 = "sha256-Q6KhpcRz8VkZRwBF4yycxJlY/EAaVGJAGHaV+Y8i7rw=";
                };
              }
              {
                name = "amd-pstate-epp-v12_5";
                patch = super.fetchpatch {
                  url = "https://lore.kernel.org/lkml/20230131090016.3970625-6-perry.yuan@amd.com/raw";
                  sha256 = "sha256-vzUmAVLAsQvbgOhpGmOrFaBGz1MEftbZ3kmIS3YLEqU=";
                };
              }
              {
                name = "amd-pstate-epp-v12_6";
                patch = super.fetchpatch {
                  url = "https://lore.kernel.org/lkml/20230131090016.3970625-7-perry.yuan@amd.com/raw";
                  sha256 = "sha256-kZuotjaFtF7yUt0g4rhj/fi1qrXzdIhtIyVRaGlkVBQ=";
                };
              }
              {
                name = "amd-pstate-epp-v12_7";
                patch = super.fetchpatch {
                  url = "https://lore.kernel.org/lkml/20230131090016.3970625-8-perry.yuan@amd.com/raw";
                  sha256 = "sha256-WehL+UIG2ANZ0cAY3p2mh8pAWbrVKTMJkTXUHISwJZE=";
                };
              }
              {
                name = "amd-pstate-epp-v12_8";
                patch = super.fetchpatch {
                  url = "https://lore.kernel.org/lkml/20230131090016.3970625-9-perry.yuan@amd.com/raw";
                  sha256 = "sha256-pFjL5nmoxDHFn6xi9faWlIzvtfuohqGdWpIKOUnXz8Y=";
                };
              }
              {
                name = "amd-pstate-epp-v12_9";
                patch = super.fetchpatch {
                  url = "https://lore.kernel.org/lkml/20230131090016.3970625-10-perry.yuan@amd.com/raw";
                  sha256 = "sha256-5qy/eY14L9e9kQlLPTTsdFYLoIW1Mbn6OQaGyDrw3HE=";
                };
              }
              {
                name = "amd-pstate-epp-v12_10";
                patch = super.fetchpatch {
                  url = "https://lore.kernel.org/lkml/20230131090016.3970625-11-perry.yuan@amd.com/raw";
                  sha256 = "sha256-1hqxRFwpSjmHdMbOXcq0gJiVPUFQOuaWS1e8fpH7hKY=";
                };
              }
              {
                name = "amd-pstate-epp-v12_11";
                patch = super.fetchpatch {
                  url = "https://lore.kernel.org/lkml/20230131090016.3970625-12-perry.yuan@amd.com/raw";
                  sha256 = "sha256-enCWKy27JC+iRm4owSqGBllCOa5QiFMg59zahLACl3w=";
                  # excludes = [ "include/linux/amd-pstate.h"];
                };
              }
            ];
            structuredExtraConfig = with lib.kernel; {
              KFENCE = yes;
            } // (lib.mkIf (config.device == "INFINITY") {
              GENERIC_CPU = no;
              GENERIC_CPU3 = yes;
            });
          });

          tlp = (super.tlp.override {
            inherit (config.boot.kernelPackages) x86_energy_perf_policy;
          }).overrideAttrs (_old: {
            version = "2023-02-25";

            src = super.fetchFromGitHub {
              owner = "linrunner";
              repo = "TLP";
              rev = "0aac593072f915c23f201e594a8b6eb27e9c94a8";
              sha256 = "sha256-F9ivInb9C2QtbEiUQbr14Es7+uwVMaYlEuUhZ7CJdlQ=";
            };
          });

          deploy-rs = inputs.deploy-rs.defaultPackage.${super.system};

          graalvm17-ee =
            let
              version = "22.3.1";
              javaVersion = "17";
              src = super.fetchurl (import ../sources/graalvm-ee-sources.nix).graalvm-ee."${javaVersion}-linux-amd64";
              meta = {
                platforms = [ "x86_64-linux" ];
                license = lib.licenses.unfree;
              };
            in
            (super.graalvmCEPackages.buildGraalvm rec {
              inherit version javaVersion src meta;
            }).overrideAttrs (_old: {
              pname = "graalvm${javaVersion}-ee";
            });

          prismlauncher = super.prismlauncher.override { jdk17 = pkgs.graalvm17-ee; jdks = with pkgs; [ jdk8 graalvm17-ee ]; };

          av1an = super.callPackage ../modules/av1an.nix { };
          cassowary = super.callPackage ../modules/cassowary.nix { };
          elfshaker = super.callPackage ../modules/elfshaker.nix { };
          ifr-extractor = super.callPackage ../modules/ifr-extractor.nix { };
        })
    ];
  };
}
