_:

{
  services.victorialogs = {
    enable = true;
    extraOptions = [
      "-retentionPeriod=14d"
    ];
  };

  services.journald.upload = {
    enable = true;
    settings.Upload.URL = "http://localhost:9428/insert/journald";
  };
}
