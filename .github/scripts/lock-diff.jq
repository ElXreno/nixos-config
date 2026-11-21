def short: .[0:7];
def day: if . then (. | strftime("%Y-%m-%d")) else "?" end;
def link($node; $old; $new):
  if $node.locked.type == "github" then
    "[`\($old | short)` → `\($new | short)`](https://github.com/\($node.locked.owner)/\($node.locked.repo)/compare/\($old)...\($new))"
  else
    "`\($old | short)` → `\($new | short)`"
  end;

.[0] as $before | .[1] as $after
| [ $after.nodes.root.inputs | to_entries[]
    | select(.value | type == "string")
    | . as $input
    | $after.nodes[$input.value] as $new
    | $before.nodes[$before.nodes.root.inputs[$input.key] // ""] as $old
    | select($old != null and $old.locked.rev != $new.locked.rev)
    | "| `\($input.key)` | \(link($new; $old.locked.rev; $new.locked.rev)) | \($old.locked.lastModified | day) → \($new.locked.lastModified | day) |"
  ] as $rows
| "| Input | Revision | Date |\n|---|---|---|\n" + ($rows | join("\n"))
