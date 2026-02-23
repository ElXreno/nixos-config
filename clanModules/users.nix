{
  inventory.instances = {
    user-root = {
      module.name = "users";
      roles.default.tags.all = { };
      roles.default.settings = {
        user = "root";
        prompt = true;
      };
    };

    user-elxreno = {
      module.name = "users";
      roles.default.tags.all = { };
      roles.default.settings = {
        user = "elxreno";
        groups = [
          "adbusers"
          "audio"
          "dialout"
          "input"
          "kvm"
          "libvirtd"
          "networkmanager"
          "sound"
          "video"
          "wheel"
          "ydotool"
        ];
      };
      roles.default.extraModules = [
        (
          {
            config,
            lib,
            pkgs,
            ...
          }:
          {
            users.users.elxreno = {
              uid = 1000;
              shell = pkgs.fish;
              linger = true;
              openssh.authorizedKeys.keys = [
                "ecdsa-sha2-nistp521 AAAAE2VjZHNhLXNoYTItbmlzdHA1MjEAAAAIbmlzdHA1MjEAAACFBAH/QtzrqDZ/isIpMslg5FJvT6BoyeqpmiaDjuzcHaIpTexaq/UK4pAdG7IYvs++6JfdfAToWeU7TnOqRj8eubfFXADNwHC3w7gHjx/w8Yq76gcRG+UU/JtUbphzs2EdWWIupaZV+nFiTSbdGlak4fnnqSLIDhRgNa3pBbvSyf2OdD02bA== elxreno@desktop.local"
                "ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAAIORnogu4KTgFE4yxS7dzJxOnuqsBYci9eNgBAnMP68G2 elxreno@gmail.com"
              ];
            };
            home-manager.users.elxreno.imports = builtins.filter (
              p: lib.hasInfix "elxreno@${config.networking.hostName}" (toString p)
            ) (lib.filesystem.listFilesRecursive ../homes);
          }
        )
      ];
    };

    user-alena = {
      module.name = "users";
      roles.default.tags.alena = { };
      roles.default.settings = {
        user = "alena";
        groups = [
          "audio"
          "input"
          "networkmanager"
          "sound"
          "video"
          "wheel"
          "ydotool"
        ];
      };
      roles.default.extraModules = [
        (
          {
            config,
            lib,
            pkgs,
            ...
          }:
          {
            users.users.alena = {
              uid = 1001;
              shell = pkgs.fish;
              linger = true;
            };
            home-manager.users.alena.imports = builtins.filter (
              p: lib.hasInfix "alena@${config.networking.hostName}" (toString p)
            ) (lib.filesystem.listFilesRecursive ../homes);
          }
        )
      ];
    };
  };
}
