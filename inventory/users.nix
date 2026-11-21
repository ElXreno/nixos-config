{
  inventory.instances =
    let
      userAccount =
        { user, uid }:
        {
          config,
          lib,
          pkgs,
          ...
        }:
        {
          users.users.${user} = {
            inherit uid;
            shell = pkgs.fish;
            linger = true;
          };
          home-manager.users.${user}.imports =
            let
              home = ../homes + "/${user}@${config.networking.hostName}/default.nix";
            in
            lib.optional (builtins.pathExists home) home;
        };
    in
    {
      user-root = {
        module.name = "users";
        roles.default = {
          tags.all = { };
          settings = {
            user = "root";
            prompt = true;
          };
        };
      };

      user-elxreno = {
        module.name = "users";
        roles.default = {
          tags.all = { };
          settings = {
            user = "elxreno";
            groups = [
              "audio"
              "dialout"
              "input"
              "kvm"
              "libvirtd"
              "networkmanager"
              "video"
              "wheel"
              "ydotool"
            ];
          };
          extraModules = [
            (userAccount {
              user = "elxreno";
              uid = 1000;
            })
            (
              { namespace, ... }:
              {
                ${namespace}.primaryUser = "elxreno";
                users.users.elxreno.openssh.authorizedKeys.keys = with import ./_ssh-keys.nix; [
                  desktop
                  gmail
                  yubikey
                ];
              }
            )
          ];
        };
      };

      user-alena = {
        module.name = "users";
        roles.default = {
          tags.alena = { };
          settings = {
            user = "alena";
            groups = [
              "audio"
              "input"
              "networkmanager"
              "video"
              "wheel"
              "ydotool"
            ];
          };
          extraModules = [
            (userAccount {
              user = "alena";
              uid = 1001;
            })
            { i18n.defaultLocale = "ru_RU.UTF-8"; }
          ];
        };
      };
    };
}
