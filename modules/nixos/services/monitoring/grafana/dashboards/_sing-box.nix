let
  datasource = {
    type = "victoriametrics-logs-datasource";
    uid = "victorialogs";
  };

  unit = ''_stream:{_SYSTEMD_UNIT="sing-box.service"}'';
  errors = ''${unit} _HOSTNAME:$host ~`^(\x1b\[[0-9;]*m)?ERROR`'';
  web = "~`dial (tcp|udp) [^ ]+:(80|443): `";
  p2p = "~`dial (tcp|udp) [^ ]+:[0-9]+: ` -~`:(80|443): `";
  hostNetwork = "~`missing default interface|no route to internet`";
  reason = "extract_regexp `(?P<reason>i/o timeout|connection refused|connection reset by peer|no route to host|network is unreachable|no route to internet|transport closed|protocol not available|context deadline exceeded|EOF)`";
  outbound = ''extract_regexp `using outbound/(?P<outbound>[a-z0-9-]+\[[^\]]+\])`'';

  series = title: gridPos: expr: legendFormat: {
    inherit title gridPos datasource;
    type = "timeseries";
    targets = [
      {
        refId = "A";
        inherit datasource expr legendFormat;
        queryType = "statsRange";
      }
    ];
    fieldConfig = {
      defaults = {
        custom = {
          drawStyle = "bars";
          fillOpacity = 60;
          stacking.mode = "normal";
        };
        unit = "short";
      };
      overrides = [ ];
    };
    options.legend = {
      displayMode = "table";
      placement = "right";
      calcs = [ "sum" ];
    };
  };

  panels = [
    (series "Web failures (80/443) by host" {
      h = 8;
      w = 12;
      x = 0;
      y = 0;
    } "${errors} ${web} | stats by (_HOSTNAME) count() as failures" "{{_HOSTNAME}}")
    (series "Web failures (80/443) by reason" {
      h = 8;
      w = 12;
      x = 12;
      y = 0;
    } "${errors} ${web} | ${reason} | stats by (reason) count() as failures" "{{reason}}")
    (series "Failures by outbound" {
      h = 8;
      w = 12;
      x = 0;
      y = 8;
    } "${errors} | ${outbound} | stats by (outbound) count() as failures" "{{outbound}}")
    (series "hysteria2 failures by reason"
      {
        h = 8;
        w = 12;
        x = 12;
        y = 8;
      }
      "${errors} ~`outbound/hysteria2` | ${reason} | stats by (reason) count() as failures"
      "{{reason}}"
    )
    (series "Host network lost" {
      h = 8;
      w = 12;
      x = 0;
      y = 16;
    } "${errors} ${hostNetwork} | stats by (_HOSTNAME) count() as failures" "{{_HOSTNAME}}")
    (series "Peer dials (P2P noise, not 80/443)" {
      h = 8;
      w = 12;
      x = 12;
      y = 16;
    } "${errors} ${p2p} | stats by (_HOSTNAME) count() as failures" "{{_HOSTNAME}}")
    {
      title = "Web and hysteria2 failures";
      type = "logs";
      inherit datasource;
      gridPos = {
        h = 12;
        w = 24;
        x = 0;
        y = 24;
      };
      targets = [
        {
          refId = "A";
          inherit datasource;
          expr = "${errors} (${web} or ~`outbound/hysteria2`) | replace_regexp (`\\x1b\\[[0-9;]*m`, ``)";
          queryType = "instant";
        }
      ];
      options = {
        showTime = true;
        wrapLogMessage = true;
        sortOrder = "Descending";
      };
    }
  ];
in
{
  title = "sing-box failures";
  uid = "sing-box-failures";
  tags = [ "sing-box" ];
  schemaVersion = 39;
  time = {
    from = "now-24h";
    to = "now";
  };
  refresh = "1m";
  templating.list = [
    {
      name = "host";
      label = "host";
      type = "query";
      inherit datasource;
      query = {
        refId = "host";
        type = "fieldValue";
        field = "_HOSTNAME";
        query = unit;
        limit = 100;
      };
      refresh = 2;
      multi = true;
      includeAll = true;
      current = { };
      options = [ ];
    }
  ];
  panels = builtins.genList (i: builtins.elemAt panels i // { id = i + 1; }) (builtins.length panels);
}
