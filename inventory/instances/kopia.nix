{
  inventory.instances.mega = {
    module.name = "kopia";
    module.input = "self";
    roles.default = {
      tags = [
        "laptop"
        "server"
      ];

      settings.s3.bucket = "kopia-backups";

      settings.users.elxreno.extraIgnore = [
        "/Android/SDK"
        "/Android/sources"
        "/backup*"
        "/Calibre Library"
        "/Downloads"
        "/go"
        "/ISOs"
        "/MEGA"
        "/Music"
        "/Pictures"
        "/tmp"
        "/Videos"
        "/VMs"
      ];

      machines.SMOK.settings.users.elxreno = {
        globalPolicy.upload = {
          maxParallelFileReads = 32;
          parallelUploadAboveSize = 268435456;
        };
        sources.datalake.path = "/mnt/datalake";
      };
    };
  };
}
