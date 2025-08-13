{ pkgs, lib, ... }:
{
  services.postgresql = {
    enable = true;
    package = pkgs.postgresql_17;
    authentication = lib.mkForce ''
      local all all trust
    '';
    settings = {
      shared_buffers = "1536MB";
      effective_cache_size = "4GB";
      maintenance_work_mem = "512MB";
      work_mem = "16MB";

      random_page_cost = 1.1;
      max_wal_size = "2GB";
      min_wal_size = "1GB";
      checkpoint_completion_target = 0.9;
      checkpoint_timeout = "15min";

      commit_delay = 1000;
      commit_siblings = 5;
    };
  };
}
