{ lib, ... }:
let
  metricsPort = 8428;
  logsPort = 9428;
in
{
  _class = "clan.service";
  manifest.name = "monitoring";
  manifest.description = "Central VictoriaMetrics, VictoriaLogs and Grafana fed by every machine";
  manifest.categories = [ "System" ];
  manifest.readme = ''
    # monitoring

    One machine in the `server` role keeps the fleet's metrics and logs:
    VictoriaMetrics on port ${toString metricsPort}, VictoriaLogs on port
    ${toString logsPort}, and Grafana as the Tailscale service
    `svc:grafana`. Both stores listen on every interface; the firewall only
    lets them through on the trusted Tailscale interface.

    Every machine in the `client` role runs the exporters and a vmagent
    that scrapes them over loopback and remote-writes to the server with a
    `host` label. The vmagent queue sits on disk, so a laptop that was
    offline delivers its backlog later. The journal goes to VictoriaLogs
    through Vector, which resumes from its acknowledged journal cursor.
  '';

  roles.server = {
    description = "Stores metrics and logs and serves Grafana.";
    perInstance = _: {
      nixosModule =
        { namespace, ... }:
        {
          ${namespace} = {
            ports = {
              victoriametrics = metricsPort;
              victorialogs = logsPort;
            };
            services.monitoring = {
              victoriametrics.enable = true;
              victorialogs.enable = true;
              grafana.enable = true;
            };
          };
        };
    };
  };

  roles.client = {
    description = "Ships metrics and the journal to the server.";
    perInstance =
      { roles, ... }:
      {
        nixosModule =
          { config, namespace, ... }:
          let
            serverName = lib.head (lib.attrNames (roles.server.machines or { }));
            serverHost = "${lib.toLower serverName}.${config.clan.core.settings.domain}";
          in
          {
            ${namespace}.services.monitoring.agent = {
              enable = true;
              metricsUrl = "http://${serverHost}:${toString metricsPort}";
              logsUrl = "http://${serverHost}:${toString logsPort}";
            };
          };
      };
  };
}
