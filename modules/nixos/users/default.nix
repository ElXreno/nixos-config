{
  config,
  lib,
  namespace,
  virtual,
  pkgs,
  ...
}:
let
  inherit (lib)
    mkOption
    literalExpression
    types
    mapAttrs
    ;

  cfg = config.${namespace}.user;

  userModule = types.submodule {
    options = {
      description = mkOption {
        type = with types; passwdEntry str;
        description = ''
          A short description of the user account, typically the
          user's full name.  This is actually the “GECOS” or “comment”
          field in {file}`/etc/passwd`.
        '';
      };
      uid = mkOption {
        type = with types; int;
        description = ''
          The account UID. If the UID is null, a free UID is picked on
          activation.
        '';
      };
      isNormalUser = mkOption {
        type = types.bool;
        default = true;
        description = ''
          Indicates whether this is an account for a “real” user.
          This automatically sets {option}`group` to `users`,
          {option}`createHome` to `true`,
          {option}`home` to {file}`/home/«username»`,
          {option}`useDefaultShell` to `true`,
          and {option}`isSystemUser` to `false`.
          Exactly one of `isNormalUser` and `isSystemUser` must be true.
        '';
      };
      extraGroups = mkOption {
        type = types.listOf types.str;
        default = [ ];
        description = "The user's auxiliary groups.";
      };
      initialPassword = mkOption {
        type = with types; nullOr str;
        default = if virtual then "1111" else null;
        description = ''
          Specifies the initial password for the user, i.e. the
          password assigned if the user does not already exist. If
          {option}`users.mutableUsers` is true, the password
          can be changed subsequently using the
          {command}`passwd` command. Otherwise, it's
          equivalent to setting the {option}`password`
          option. The same caveat applies: the password specified here
          is world-readable in the Nix store, so it should only be
          used for guest accounts or passwords that will be changed
          promptly.
        '';
      };
      hashedPasswordFile = mkOption {
        type = with types; nullOr str;
        default = null;
        description = ''
          The full path to a file that contains the hash of the user's
          password. The password file is read on each system activation. The
          file should contain exactly one line, which should be the password in
          an encrypted form that is suitable for the `chpasswd -e` command.
        '';
      };
      linger = mkOption {
        type = with types; nullOr bool;
        example = true;
        default = null;
        description = ''
          Whether to enable or disable lingering for this user.  Without
          lingering, user units will not be started until the user logs in,
          and may be stopped on logout depending on the settings in
          `logind.conf`.

          By default, NixOS will not manage lingering, new users will default
          to not lingering, and lingering can be configured imperatively using
          `loginctl enable-linger` or `loginctl disable-linger`. Setting
          this option to `true` or `false` is the declarative equivalent of
          running `loginctl enable-linger` or `loginctl disable-linger`
          respectively.
        '';
      };
      shell = mkOption {
        type = types.nullOr (types.either types.shellPackage (types.passwdEntry types.path));
        default = pkgs.fish;
        defaultText = literalExpression "pkgs.fish";
        example = literalExpression "pkgs.bashInteractive";
        description = ''
          The path to the user's shell. Can use shell derivations,
          like `pkgs.bashInteractive`. Don’t
          forget to enable your shell in
          `programs` if necessary,
          like `programs.zsh.enable = true;`.
        '';
      };
      openssh.authorizedKeys = {
        keys = lib.mkOption {
          type = lib.types.listOf lib.types.singleLineStr;
          default = [ ];
          description = ''
            A list of verbatim OpenSSH public keys that should be added to the
            user's authorized keys. The keys are added to a file that the SSH
            daemon reads in addition to the the user's authorized_keys file.
            You can combine the `keys` and
            `keyFiles` options.
            Warning: If you are using `NixOps` then don't use this
            option since it will replace the key required for deployment via ssh.
          '';
        };
      };
    };
  };
in
{
  options.${namespace}.user = with types; {
    users = mkOption {
      default = { };
      type = with types; attrsOf userModule;
      example = {
        alice = {
          uid = 1234;
          description = "Alice Q. User";
          extraGroups = [ "wheel" ];
        };
      };
      description = ''
        Additional user accounts to be created automatically by the system.
        This can also be used to set options for root.
      '';
    };
  };

  config = {
    users = {
      mutableUsers = false;
      users = mapAttrs (
        name: user:
        let
          mainUser = user.uid == 1000;
          forMainUser = cfg: cfg && mainUser;
        in
        {
          inherit name;
          inherit (user)
            description
            uid
            isNormalUser
            initialPassword
            hashedPasswordFile
            linger
            shell
            openssh
            ;

          extraGroups =
            (lib.optionals user.isNormalUser (
              [
                "wheel"
                "audio"
                "sound"
                "video"
                "input"
              ]
              ++ lib.optional config.networking.networkmanager.enable "networkmanager"
              # I know that is not a full restriction, but anyway let's do that
              ++ lib.optional (forMainUser config.${namespace}.programs.adb.enable) "adbusers"
              ++ lib.optionals (forMainUser config.virtualisation.libvirtd.enable) [
                "kvm"
                "libvirtd"
              ]
            ))
            ++ user.extraGroups;
        }
      ) cfg.users;
    };

    home-manager = {
      useGlobalPkgs = true;
      useUserPackages = true;
      backupFileExtension = ".bak";
    };
  };
}
