def with_host:
  gsub("(?<m>[a-zA-Z_:][a-zA-Z0-9_:]*)\\{"; "\(.m){host=~\"$host\",")
  | reduce $bare[] as $b (.; gsub("\\b\($b)\\b(?![{a-zA-Z0-9_:])"; "\($b){host=~\"$host\"}"));

.templating.list = [
  {
    name: "host",
    label: "host",
    type: "query",
    datasource: { type: "prometheus", uid: "${DS_PROMETHEUS}" },
    definition: "label_values(\($metric), host)",
    query: {
      qryType: 1,
      query: "label_values(\($metric), host)",
      refId: "PrometheusVariableQueryEditor-VariableQuery"
    },
    refresh: 1,
    regex: "",
    options: [],
    current: {}
  }
] + .templating.list
| (.. | objects | select(has("expr")) | .expr) |= with_host
