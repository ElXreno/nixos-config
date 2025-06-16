{
  lib,
  config,
  inputs,
  pkgs,
  ...
}:
let
  cfg = config.device;

  hasService =
    name:
    builtins.any (
      user:
      let
        allUserServices = cfg.defaultUserServices ++ cfg.users.${user}.services;
      in
      builtins.elem name allUserServices
    ) cfg.activeUsers;

  hasProgram =
    name:
    builtins.any (
      user:
      let
        allUserPrograms = cfg.defaultUserPrograms ++ cfg.users.${user}.programs;
      in
      builtins.elem name allUserPrograms
    ) cfg.activeUsers;
in
{
  options = {
    device = {
      hostname = lib.mkOption {
        type = lib.types.nullOr lib.types.str;
        default = null;
      };

      isPrimary = lib.mkEnableOption "isPrimary";

      isDesktop = lib.mkEnableOption "isDesktop";
      isLaptop = lib.mkEnableOption "isLaptop";
      isServer = lib.mkEnableOption "isServer";

      isLegacy = lib.mkEnableOption "isLegacy";

      laptop = {
        manufacturer = {
          Fujitsu = lib.mkEnableOption "Fujitsu";
          Asus = lib.mkEnableOption "Asus";
          Dell = lib.mkEnableOption "Dell";
          Honor = lib.mkEnableOption "Honor";
        };
        model = lib.mkOption {
          type = lib.types.nullOr lib.types.str;
          default = null;
        };
      };

      cpu = {
        amd = lib.mkEnableOption "amd";
        intel = lib.mkEnableOption "intel";
      };

      gpu = {
        nvidia = lib.mkEnableOption "nvidia";
        nvidiaLegacy = lib.mkEnableOption "nvidiaLegacy";
        amd = lib.mkEnableOption "amd";
        intel = lib.mkEnableOption "intel";
      };

      network = {
        hasWirelessCard = lib.mkEnableOption "hasWirelessCard";
        wirelessCard = lib.mkOption {
          type = lib.types.nullOr (
            lib.types.enum [
              "RTL8852BE" # Realtek
              "AX200" # Intel
            ]
          );
          default = null;
        };
      };

      activeUsers = lib.mkOption {
        type = lib.types.listOf lib.types.str;
        default = builtins.filter (userName: cfg.users.${userName}.enable) (builtins.attrNames cfg.users);
      };

      defaultUserPrograms = lib.mkOption {
        type = lib.types.listOf lib.types.str;
        default = [ ];
      };

      defaultUserServices = lib.mkOption {
        type = lib.types.listOf lib.types.str;
        default = [ ];
      };

      users =
        let
          mkUser = name: {
            enable = lib.mkEnableOption name // {
              default = name == "elxreno";
            };
            programs = lib.mkOption {
              type = lib.types.listOf lib.types.str;
              default = cfg.defaultUserPrograms;
            };
            services = lib.mkOption {
              type = lib.types.listOf lib.types.str;
              default = cfg.defaultUserServices;
            };
          };
        in
        {
          elxreno = mkUser "elxreno";
          alena = mkUser "alena";
        };

      # TODO: implement this
      # desktopEnvironment = lib.mkOption {
      #   type = lib.types.nullOr (
      #     lib.types.enum [
      #       "plasma"
      #       "hyprland"
      #     ]
      #   );
      #   default = lib.mkIf (!cfg.isServer) "plasma";
      # };
    };
  };

  imports = [
    inputs.home-manager.nixosModules.home-manager
  ];

  config = {
    device = {
      defaultUserPrograms = [
        "firefox"
        "fish"
        "git"
        "helix"
        "htop"
      ];
      users.elxreno = {
        programs = [
          "direnv"
          "gpg"
          "mangohud"
          "mpv"
          "nix-index"
          "ssh"
          "starship"
          "vscodium"
        ];
        services = [
          "activitywatch"
          "gpg-agent"
          "kdeconnect"
          "syncthing"
        ];
      };
    };

    networking.firewall = lib.mkMerge [
      (lib.mkIf (hasService "syncthing") {
        allowedTCPPorts = [ 22000 ]; # TCP based sync protocol traffic
        allowedUDPPorts = [
          22000
          21027
        ]; # QUIC based sync protocol traffic & for discovery broadcasts
      })
      (lib.mkIf (hasService "kdeconnect") {
        allowedTCPPortRanges = [
          {
            from = 1714;
            to = 1764;
          }
        ];
        allowedUDPPortRanges = [
          {
            from = 1714;
            to = 1764;
          }
        ];
      })
    ];

    services.gnome.gnome-keyring.enable = hasProgram "vscodium";

    home-manager = {
      extraSpecialArgs = {
        inherit inputs pkgs;
        systemConfig = config;
      };

      users = builtins.listToAttrs (
        builtins.map (user: {
          name = user;
          value =
            let
              allUserPrograms = cfg.defaultUserPrograms ++ cfg.users.${user}.programs;
              allUserServices = cfg.defaultUserServices ++ cfg.users.${user}.services;

              homeModulesPath = "${inputs.self}/profiles/home";

              getModules =
                path:
                if builtins.pathExists (homeModulesPath + "/${path}") then
                  builtins.readDir (homeModulesPath + "/${path}")
                else
                  { };

              availablePrograms = builtins.attrNames (getModules "programs");

              getPackageName = prog: prog.pname or prog.name or (builtins.baseNameOf prog);

              programsWithModules = builtins.filter (
                prog:
                let
                  packageName = getPackageName prog;
                in
                builtins.any (moduleName: builtins.match "${packageName}.nix" moduleName != null) availablePrograms
              ) allUserPrograms;

              programsWithoutModules = builtins.map (p: pkgs.${p}) (
                builtins.filter (prog: !(builtins.elem prog programsWithModules)) allUserPrograms
              );
            in
            {
              imports =
                (builtins.map (prog: homeModulesPath + "/programs/${getPackageName prog}.nix") programsWithModules)
                ++ (builtins.map (svc: homeModulesPath + "/services/${svc}.nix") allUserServices);

              home = {
                packages = programsWithoutModules;
              };
            };
        }) cfg.activeUsers
      );
    };
  };
}
