def safe: gsub("[\u0000-\u0009\u000b-\u001f\u007f-\u009f]"; "");
def clip($n): if length > $n then .[:$n] + "..." else . end;
def flat: gsub("\\s+"; " ") | safe;
def brief:
  if type != "object" then tostring
  elif .command then .command
  elif .url then .url
  elif .file_path then .file_path
  elif .pattern then .pattern
  elif .description then .description
  elif .prompt then .prompt
  else tostring end;

fromjson? |
(if .parent_tool_use_id != null or .isSidechain == true then "    " else "" end) as $i |
if .type == "system" and .subtype == "init" then
  "== \(.model), \(.tools | length) tools"
elif .type == "assistant" then
  .message.content[]? |
  if .type == "text" then $i + (.text | gsub("\n+$"; "") | safe)
  elif .type == "tool_use" then $i + "-> \(.name) " + (.input | brief | tostring | flat | clip(200))
  else empty end
elif .type == "user" and (.message.content | type) == "string" then
  $i + "you> " + (.message.content | flat | clip(400))
elif .type == "user" then
  .message.content[]? | select(type == "object" and .type == "tool_result") |
  $i + "   <- "
    + ((.content | if type == "array" then map(.text? // "") | join(" ") else tostring end) | flat | clip(160))
    + (if .is_error then " [error]" else "" end)
elif .type == "result" then
  "== \(.subtype), \(.num_turns) turns, \((.duration_ms // 0) / 1000 | floor) s"
else empty end
