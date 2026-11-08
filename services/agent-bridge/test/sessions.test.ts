import assert from "node:assert/strict";
import { test } from "node:test";
import type { SDKSessionInfo, SessionMessage } from "@anthropic-ai/claude-agent-sdk";
import { SessionReader, toTurns } from "../src/sessions.ts";

function msg(type: "user" | "assistant", content: unknown, timestamp = "2026-11-07T08:00:00.000Z"): SessionMessage {
  return { type, uuid: crypto.randomUUID(), session_id: "s1", message: { role: type, content }, parent_tool_use_id: null, parent_agent_id: null, timestamp } as SessionMessage;
}

const conversation: SessionMessage[] = [
  msg("user", "PDFを読んで<system-reminder>内部向け</system-reminder>"),
  msg("assistant", [{ type: "thinking", thinking: "..." }]),
  msg("assistant", [{ type: "tool_use", name: "Bash", input: { command: "ls  -la\n docs" } }]),
  msg("user", [{ type: "tool_result", tool_use_id: "x", content: "a.pdf" }]),
  msg("assistant", [{ type: "text", text: "読みました。" }], "2026-11-07T08:01:00.000Z"),
  msg("user", "要約して"),
  msg("assistant", [{ type: "text", text: "要約です。" }]),
  msg("user", "<command-name>/clear</command-name>"),
  msg("user", "続けて"),
  msg("assistant", [{ type: "tool_use", name: "StructuredOutput", input: { summary: "続けました。", details: [] } }]),
];

test("toTurns は発話と返答の組にまとめ、ツールは1行の要約にする", () => {
  assert.deepEqual(toTurns(conversation), [
    { role: "user", text: "PDFを読んで", at: "2026-11-07T08:00:00.000Z" },
    { role: "assistant", text: "読みました。", tools: ["[Bash] ls -la docs"], at: "2026-11-07T08:01:00.000Z" },
    { role: "user", text: "要約して", at: "2026-11-07T08:00:00.000Z" },
    { role: "assistant", text: "要約です。", tools: undefined, at: "2026-11-07T08:00:00.000Z" },
    { role: "user", text: "続けて", at: "2026-11-07T08:00:00.000Z" },
    { role: "assistant", text: "続けました。", tools: undefined, at: "2026-11-07T08:00:00.000Z" },
  ]);
});

const info: SDKSessionInfo = {
  sessionId: "s1",
  summary: "PDFの要約",
  lastModified: Date.parse("2026-11-07T09:00:00Z"),
  createdAt: Date.parse("2026-11-07T08:00:00Z"),
  firstPrompt: "PDFを読んで",
  gitBranch: "main",
  tag: "smart-speaker",
};

function reader(running = new Set<string>(), cwd = "/work") {
  const calls: unknown[] = [];
  const api = {
    listSessions: async (options: unknown) => (calls.push(options), [info]),
    getSessionInfo: async (id: string) => (id === "s1" ? { ...info, cwd } : undefined),
    getSessionMessages: async () => conversation,
  };
  return { reader: new SessionReader(api, "/work", { runningTask: (id: string) => (running.has(id) ? { id: "t1" } : undefined), pendingCount: () => 1 }), calls };
}

test("list は作業ディレクトリだけを対象にし、実行中かどうかを足す", async () => {
  const { reader: r, calls } = reader(new Set(["s1"]));
  const [s] = await r.list(5);
  assert.deepEqual(calls, [{ dir: "/work", limit: 5, includeWorktrees: false }]);
  assert.equal(s.title, "PDFの要約");
  assert.equal(s.created_at, "2026-11-07T08:00:00.000Z");
  assert.equal(s.from_smart_speaker, true);
  assert.equal(s.running, true);
  assert.equal(s.task_id, "t1");
  assert.equal(s.pending_permissions, 1);
});

test("messages の offset は新しい側から数える", async () => {
  const { reader: r } = reader();
  const latest = await r.messages("s1", 2, 0);
  assert.equal(latest.total, 6);
  assert.deepEqual(latest.messages.map((m) => m.text), ["続けて", "続けました。"]);
  const older = await r.messages("s1", 2, 4);
  assert.deepEqual(older.messages.map((m) => m.text), ["PDFを読んで", "読みました。"]);
  assert.deepEqual((await r.messages("s1", 2, 10)).messages, []);
});

test("ないセッションは404にする", async () => {
  const { reader: r } = reader();
  await assert.rejects(r.messages("missing", 2, 0), { status: 404 });
});

test("worktreeなど別の場所で始めたセッションは、作業ディレクトリのセッションとして扱わない", async () => {
  assert.equal(await reader(new Set(), "/work/").reader.exists("s1"), true);
  assert.equal(await reader(new Set(), "/work/.claude/worktrees/x").reader.exists("s1"), false);
});
