// flock integration for Pi — installed by `flock install pi`.
// Reports agent state (working / needs input / done) to the flock daemon so the
// sidebar, notifications and terminal header can show it. It only activates
// inside a flock terminal and never blocks or fails Pi: every send has a short
// timeout and errors are ignored.
// FLOCK_INTEGRATION_VERSION=1
// @ts-nocheck

import net from "node:net";
import path from "node:path";

const pane = process.env.TMUX_PANE;
const socketPath =
  process.env.FLOCK_SOCKET ||
  path.join(process.env.XDG_RUNTIME_DIR || "/tmp", "flock", "flock.sock");

// Only report from panes on flock's private tmux server ($TMUX = "<socket>,<pid>,<idx>").
function enabled(): boolean {
  const tmux = process.env.TMUX || "";
  return !!pane && path.basename(tmux.split(",")[0] || "") === (process.env.FLOCK_TMUX || "flock");
}

let sessionId = "";

function send(name: string, fields: Record<string, string> = {}): Promise<void> {
  if (!enabled() || !sessionId) return Promise.resolve();
  const event = { agent: "pi", name, pane, session_id: sessionId, ...fields };
  return new Promise((resolve) => {
    let done = false;
    const finish = () => {
      if (done) return;
      done = true;
      socket.destroy();
      resolve();
    };
    const socket = net.createConnection(socketPath);
    socket.on("error", finish);
    socket.on("connect", () => socket.write(JSON.stringify({ op: "event", event }) + "\n"));
    socket.on("data", finish);
    socket.on("end", finish);
    setTimeout(finish, 300).unref?.();
  });
}

function summarise(toolName: string, args: any): string {
  const a = args || {};
  const detail = a.command || a.path || a.file_path || a.pattern || a.url || "";
  return `${toolName} ${String(detail).split("\n")[0]}`.trim().slice(0, 80);
}

function lastAssistantText(messages: any[]): string {
  for (let i = (messages || []).length - 1; i >= 0; i--) {
    const m = messages[i];
    if (m?.role !== "assistant") continue;
    const text = (m.content || [])
      .filter((c: any) => c?.type === "text")
      .map((c: any) => c.text)
      .join(" ")
      .trim();
    if (text) return text;
  }
  return "";
}

export default function (pi) {
  if (!enabled()) return;

  let root = false;
  let lastText = "";

  pi.on("session_start", async (event, ctx) => {
    if (ctx?.mode !== "tui") return; // print/RPC modes have no terminal to show
    root = true;
    try {
      sessionId = ctx.sessionManager.getSessionId() || "";
    } catch {
      sessionId = "";
    }
    await send("start", { source: event?.reason === "resume" ? "resume" : event?.reason || "startup" });
  });

  pi.on("before_agent_start", (event) => {
    if (root) void send("prompt", { prompt: String(event?.prompt || "").slice(0, 200) });
  });

  pi.on("tool_execution_start", (event) => {
    if (root) void send("tool", { activity: summarise(event?.toolName, event?.args) });
  });

  pi.on("ui_prompt_start", (event) => {
    if (root) void send("input", { detail: String(event?.title || "Pi is asking you something") });
  });

  pi.on("ui_prompt_end", () => {
    if (root) void send("resume_work");
  });

  pi.on("agent_end", (event) => {
    lastText = lastAssistantText(event?.messages);
  });

  pi.on("agent_settled", (_event, ctx) => {
    if (!root || ctx?.isIdle?.() === false) return;
    const text = lastText.replace(/\s+/g, " ").trim();
    // A turn that ends on a question is waiting for the user, not done.
    if (text.endsWith("?")) void send("input", { detail: text.slice(-160) });
    else void send("stop", { detail: text.slice(0, 160) });
  });

  pi.on("session_shutdown", async () => {
    if (root) await send("end");
  });
}
