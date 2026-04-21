Analyze previous Claude Code conversation logs for this project to extract patterns, update CLAUDE.md, and recommend subagents.

## Step 1: Extract conversations

Preprocess all conversation logs at once via bash. The project-specific conversation directory is at `~/.claude/projects/` with the project path encoded as dash-separated segments (e.g., `-home-user-projects-myrepo/`). Find the correct directory by matching the current working directory.

```bash
# Derive the project conversation dir from $PWD
PROJECT_DIR=$(echo "$PWD" | sed 's|/|-|g; s|^|/home/'$USER'/.claude/projects/|')
nix shell nixpkgs#python3 -c python3 << 'PYEOF'
import json, re, os

project_dir = os.environ["PROJECT_DIR"]
out = []
for fname in sorted(os.listdir(project_dir)):
    if not fname.endswith(".jsonl"): continue
    fpath = os.path.join(project_dir, fname)
    session_lines = []
    with open(fpath) as f:
        for line in f:
            line = line.strip()
            if not line: continue
            try:
                obj = json.loads(line)
            except: continue
            msg_type = obj.get("type","")
            if msg_type == "human":
                msg = obj.get("message",{})
                content = msg.get("content","")
                if isinstance(content, list):
                    texts = [c.get("text","") for c in content if isinstance(c, dict) and c.get("type")=="text"]
                    content = " ".join(texts)
                if not isinstance(content, str): continue
                clean = re.sub(r"<[^>]*>", "", content).strip()
                if clean and len(clean) > 3:
                    session_lines.append(f"USER: {clean[:500]}")
            elif msg_type == "assistant":
                msg = obj.get("message",{})
                content = msg.get("content","")
                if isinstance(content, list):
                    texts = [c.get("text","") for c in content if isinstance(c, dict) and c.get("type")=="text"]
                    content = " ".join(texts)
                if not isinstance(content, str): continue
                content = content.strip()
                if content and len(content) > 10:
                    session_lines.append(f"ASST: {content[:300]}")
    if session_lines:
        out.append(f"=== SESSION: {fname} ===")
        out.extend(session_lines)
        out.append("")

with open("/tmp/conversations-extracted.txt", "w") as f:
    f.write("\n".join(out))
print(f"Extracted {len(out)} lines from sessions with content")
PYEOF
```

Then split the output into ~6 equal chunks by session boundaries using `nix shell nixpkgs#python3 -c python3` and write each to `/tmp/conv-chunk-{1..6}.txt`.

## Step 2: Parallel analysis

Launch 6 subagents in parallel, one per chunk. Each agent reads its chunk file and reports:
- **Summary** per session (1-3 sentences)
- **User preferences/corrections** ("don't do X", "always do Y", corrections to Claude's approach)
- **Pain points** (things that went wrong, workarounds)
- **Automation ideas** (repetitive tasks, hooks, subagents mentioned)
- **Cross-chunk patterns**

## Step 3: Synthesize

After all agents return, combine their findings into:

### 3a. User preferences & corrections
Group all corrections and preferences. Deduplicate against existing memories in the project memory directory. Save new ones as memory files.

### 3b. CLAUDE.md updates
Compare findings against current `CLAUDE.md`. Propose additions for:
- New conventions discovered from conversations
- New common pitfalls (things Claude got wrong or that caused debugging)
- Updated cluster topology or tech stack changes
- New patterns specific to the project

Show the diff to the user before applying.

### 3c. Subagent recommendations
Identify repetitive multi-step tasks from the conversations that could be automated as `/project:` commands. For each, propose:
- Command name
- What it does (1-2 sentences)
- Why (how often it came up, time saved)

Cross-reference against existing commands in `.claude/commands/` — don't recommend what already exists. Ask the user which ones to create.

## Rules
- Do NOT write CLAUDE.md changes or create commands without user confirmation
- Deduplicate against existing memories and commands
- Focus on patterns that span multiple sessions, not one-off tasks
- Keep CLAUDE.md additions concise — pitfalls and conventions, not prose
