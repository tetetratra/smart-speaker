import assert from "node:assert/strict";
import { test } from "node:test";
import type { Options, SDKMessage, SDKResultMessage } from "@anthropic-ai/claude-agent-sdk";
import type { EventInput } from "../src/protocol.ts";
import { PermissionBroker } from "../src/permissions.ts";
import { TaskManager, interpret, childEnv } from "../src/tasks.ts";

const SESSION = "11111111-2222-4333-8444-555555555555";

function success(structured: unknown): SDKResultMessage {
  return { type: "result", subtype: "success", is_error: false, result: "raw", structured_output: structured, permission_denials: [] } as unknown as SDKResultMessage;
}

const WORKDIRS = [
  { name: "work", path: "/work" },
  { name: "speaker", path: "/speaker" },
];

// found はセッションを始めた作業ディレクトリ。null なら見つからないセッションとして扱う。
function setup(run: (options: Options) => AsyncIterable<SDKMessage>, found: (typeof WORKDIRS)[number] | null = WORKDIRS[1]) {
  const events: EventInput[] = [];
  const queries: Options[] = [];
  const tags: string[] = [];
  let done!: () => void;
  const finished = new Promise<void>((resolve) => (done = resolve));
  const notify = async (event: EventInput) => {
    events.push(event);
    if (event.type !== "permission_request") done();
    return true;
  };
  const broker = new PermissionBroker(notify, 1000);
  const manager = new TaskManager({
    query: ({ options }) => (queries.push(options!), run(options!)),
    tagSession: async (id, tag, options) => void tags.push(`${id}:${tag}:${options?.dir}`),
    findSession: async () => found ?? undefined,
    broker,
    notify,
    workdirs: WORKDIRS,
    maxTasks: 1,
    taskTimeoutMs: 60_000,
    log: () => {},
  });
  return { manager, broker, events, queries, tags, finished };
}

test("新しい依頼は sessionId を発行し、完了したらタグを付けて通知する", async () => {
  const { manager, events, queries, tags, finished } = setup(async function* () {
    yield success({ summary: "終わりました", details: ["a.txt を作成"] });
  });
  const accepted = await manager.start("ファイルを作って");
  await finished;
  const options = queries[0];
  assert.equal(options.sessionId, accepted.session_id);
  assert.equal(options.resume, undefined);
  assert.equal(options.permissionMode, "auto");
  assert.equal(options.cwd, "/work");
  assert.deepEqual(tags, [`${accepted.session_id}:smart-speaker:/work`]);
  assert.equal(accepted.workdir, "work");
  assert.deepEqual(events[0], { type: "completed", summary: "終わりました", details: ["a.txt を作成"], task_id: accepted.task_id, session_id: accepted.session_id, request: "ファイルを作って" });
  assert.deepEqual(options.disallowedTools, ["AskUserQuestion", "EnterPlanMode", "ExitPlanMode"]);
});

test("続きの依頼は resume を使い、タグは付け直さない", async () => {
  const { manager, queries, tags, finished } = setup(async function* () {
    yield success({ summary: "ok", details: [] });
  });
  // 続きの依頼は、指定した名前ではなくセッションを始めた場所で再開する。
  const accepted = await manager.start("続き", SESSION, "work");
  await finished;
  assert.equal(accepted.session_id, SESSION);
  assert.equal(accepted.workdir, "speaker");
  assert.equal(queries[0].cwd, "/speaker");
  assert.equal(queries[0].resume, SESSION);
  assert.equal(queries[0].sessionId, undefined);
  assert.deepEqual(tags, []);
});

test("canUseTool の承認依頼は、作業とセッションのIDを付けて通知する", async () => {
  const { manager, broker, events, finished } = setup(async function* (options) {
    const result = await options.canUseTool!("Bash", { command: "rm a" }, { signal: new AbortController().signal, toolUseID: "u1", title: "Claude wants to run rm a" } as never);
    yield success({ summary: result?.behavior, details: [] });
  });
  const accepted = await manager.start("消して");
  await new Promise((resolve) => setImmediate(resolve));
  const request = events[0] as Extract<EventInput, { type: "permission_request" }>;
  assert.equal(request.type, "permission_request");
  assert.equal(request.task_id, accepted.task_id);
  assert.equal(request.title, "Claude wants to run rm a");
  assert.equal(manager.pendingCount(accepted.session_id), 1);
  broker.answer(request.request_id, true);
  await finished;
  assert.equal((events[1] as { summary: string }).summary, "allow");
});

test("入力の誤り・実行中・上限・存在しないセッションを断る", async () => {
  let release!: () => void;
  const blocker = new Promise<void>((resolve) => (release = resolve));
  const { manager, finished } = setup(async function* () {
    await blocker;
    yield success({ summary: "ok", details: [] });
  });
  await assert.rejects(manager.start("  "), { status: 400 });
  await assert.rejects(manager.start("x", "not-a-uuid"), { status: 400 });
  await manager.start("x", SESSION);
  await assert.rejects(manager.start("y", SESSION), { status: 409 });
  await assert.rejects(manager.start("z"), { status: 429 });
  release();
  await finished;

  const missing = setup(async function* () {}, null);
  await assert.rejects(missing.manager.start("x", SESSION), { status: 404 });
});

test("作業ディレクトリは名前で選び、ない名前は候補つきで断る", async () => {
  const { manager, queries, tags, finished } = setup(async function* () {
    yield success({ summary: "ok", details: [] });
  });
  await assert.rejects(manager.start("x", undefined, "none"), { status: 400, message: "作業ディレクトリ「none」はありません。候補: work, speaker" });
  const accepted = await manager.start("x", undefined, "speaker");
  await finished;
  assert.equal(accepted.workdir, "speaker");
  assert.equal(queries[0].cwd, "/speaker");
  assert.deepEqual(tags, [`${accepted.session_id}:smart-speaker:/speaker`]);
});

test("中止すると失敗として通知する", async () => {
  const { manager, events, finished } = setup(async function* (options) {
    await new Promise((resolve) => options.abortController!.signal.addEventListener("abort", resolve));
    throw new Error("aborted");
  });
  const accepted = await manager.start("長い作業");
  manager.cancel(accepted.task_id);
  await finished;
  assert.deepEqual(events[0], { type: "failed", error: "作業を中止しました", task_id: accepted.task_id, session_id: accepted.session_id, request: "長い作業" });
  assert.throws(() => manager.cancel("unknown"), { status: 404 });
  assert.throws(() => manager.cancelSession(accepted.session_id), { status: 404 });
});

test("セッションIDでも実行中の作業を止められる", async () => {
  const { manager, events, finished } = setup(async function* (options) {
    await new Promise((resolve) => options.abortController!.signal.addEventListener("abort", resolve));
    throw new Error("aborted");
  });
  const accepted = await manager.start("長い作業");
  assert.equal(manager.runningTask(accepted.session_id)?.id, accepted.task_id);
  manager.cancelSession(accepted.session_id);
  await finished;
  assert.equal(events[0].type, "failed");
  assert.equal(manager.runningTask(accepted.session_id), undefined);
});

test("interpret は失敗や構造化出力の欠落を扱う", () => {
  assert.deepEqual(interpret(undefined, new Error("aborted"), true, true), { type: "failed", error: "時間の上限に達したので作業を止めました" });
  assert.deepEqual(interpret(undefined, new Error("boom"), false), { type: "failed", error: "Error: boom" });
  const error = { type: "result", subtype: "error_max_turns", is_error: true, errors: ["too many"], permission_denials: [] } as unknown as SDKResultMessage;
  assert.deepEqual(interpret(error, undefined, false), { type: "failed", error: "too many" });
  const denied = { ...success(undefined), permission_denials: [{ tool_name: "Bash", tool_use_id: "u", tool_input: { command: "rm" } }] } as SDKResultMessage;
  assert.deepEqual(interpret(denied, undefined, false), { type: "completed", summary: "raw", details: [], permission_denials: ['Bash: {"command":"rm"}'] });
});

test("childEnv は Claude 系・ブリッジ用・シークレットらしい環境変数を渡さない", () => {
  const source = {
    HOME: "/h",
    PATH: "/bin",
    KEYBOARD_LAYOUT: "jis",
    TOKEN_COUNT: "3",
    ANTHROPIC_BASE_URL: "x",
    CLAUDECODE: "1",
    CLAUDE_CODE_ENTRYPOINT: "y",
    AGENT_BRIDGE_LISTEN: "127.0.0.1:8787",
    AGENT_BRIDGE_TOKEN: "t",
    OPENAI_API_KEY: "k",
    SWITCHBOT_TOKEN: "t",
    GOOGLE_CLIENT_SECRET: "s",
    DB_PASSWORD: "p",
  };
  // 名前の途中に TOKEN や KEY を含むだけの変数は残す。
  assert.deepEqual(childEnv(source), { HOME: "/h", PATH: "/bin", KEYBOARD_LAYOUT: "jis", TOKEN_COUNT: "3" });
});
