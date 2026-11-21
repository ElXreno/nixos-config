import type { ExtensionAPI, ExtensionContext } from "@earendil-works/pi-coding-agent";
import { spawn } from "node:child_process";

const GUARD = "@guard@";
const CONFIG = "@config@";
const POWER = "@power@" === "1";
const TOOLS = new Set(["bash", "process", "edit", "write"]);

type Answer = {
  block?: string;
  confirm?: { message: string; declined: string };
  patch?: { command: string };
  append?: string;
};

function hook(payload: object, signal?: AbortSignal): Promise<Answer> {
  return new Promise((done, fail) => {
    const child = spawn(GUARD, ["--config", CONFIG, "hook"], { stdio: ["pipe", "pipe", "pipe"], signal, timeout: 10000 });
    let stdout = "";
    let stderr = "";
    child.stdout.on("data", (d) => (stdout += d));
    child.stderr.on("data", (d) => (stderr += d));
    child.on("error", fail);
    child.on("close", (code) => {
      if (code !== 0) return fail(new Error(`exited ${code}: ${stderr.trim()}`));
      try {
        done(JSON.parse(stdout));
      } catch (err) {
        fail(err);
      }
    });
    child.stdin.end(JSON.stringify(payload));
  });
}

export default function (pi: ExtensionAPI) {
  const advice = new Map<string, string>();

  pi.on("tool_call", async (event, ctx) => {
    if (!TOOLS.has(event.toolName)) return;
    const sessionId = ctx.sessionManager.getSessionId();
    let answer: Answer;
    try {
      answer = await hook(
        {
          event: "tool_call",
          toolName: event.toolName,
          toolCallId: event.toolCallId,
          input: event.input,
          cwd: ctx.cwd,
          sessionId,
          hasUI: ctx.hasUI,
        },
        ctx.signal,
      );
    } catch (err) {
      ctx.ui.notify(`agent-guard did not run: ${err}`, "warning");
      return;
    }
    if (answer.block) return { block: true, reason: answer.block };
    if (answer.append) advice.set(event.toolCallId, answer.append);
    if (answer.confirm) {
      const approved = await ctx.ui.confirm("agent-guard", answer.confirm.message);
      hook({ event: "dialog_result", toolName: event.toolName, toolCallId: event.toolCallId, sessionId, approved }).catch((err) =>
        ctx.ui.notify(`agent-guard did not record the answer: ${err}`, "warning"),
      );
      if (!approved) {
        advice.delete(event.toolCallId);
        return { block: true, reason: answer.confirm.declined };
      }
    }
    if (answer.patch) Object.assign(event.input as object, answer.patch);
  });

  async function power(event: object, ctx: ExtensionContext) {
    if (!POWER) return undefined;
    try {
      const answer = await hook({ ...event, cwd: ctx.cwd, sessionId: ctx.sessionManager.getSessionId() }, ctx.signal);
      return answer.append;
    } catch (err) {
      ctx.ui.notify(`agent-guard did not run: ${err}`, "warning");
      return undefined;
    }
  }

  pi.on("tool_result", async (event, ctx) => {
    const texts: string[] = [];
    const text = advice.get(event.toolCallId);
    if (text !== undefined) {
      advice.delete(event.toolCallId);
      texts.push(text);
    }
    const change = await power({ event: "tool_result", toolName: event.toolName, toolCallId: event.toolCallId }, ctx);
    if (change) texts.push(change);
    if (texts.length === 0) return;
    return {
      content: [...event.content, ...texts.map((t) => ({ type: "text" as const, text: t }))],
      structuredContent: event.structuredContent,
    };
  });

  pi.on("before_agent_start", async (event, ctx) => {
    if (event.messageType !== "user") return;
    const change = await power({ event: "prompt" }, ctx);
    if (!change) return;
    return { message: { customType: "agent-guard", content: change, display: true } };
  });

  pi.on("tool_execution_end", async (event) => {
    advice.delete(event.toolCallId);
  });
}
