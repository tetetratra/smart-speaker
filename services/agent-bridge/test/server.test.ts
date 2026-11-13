import assert from "node:assert/strict";
import type { AddressInfo } from "node:net";
import { after, before, test } from "node:test";
import { HttpError } from "../src/protocol.ts";
import { createBridgeServer } from "../src/server.ts";

const TOKEN = "test-token-0123456789";
const calls: unknown[][] = [];
const server = createBridgeServer({
  token: TOKEN,
  tasks: {
    start: async (input: string, sessionId?: string, workdir?: string, context?: Record<string, unknown>) => (calls.push(["start", input, sessionId, workdir, ...(context === undefined ? [] : [context])]), { task_id: "t1", session_id: "s1", workdir: "work" }),
    cancel: (id: string) => {
      if (id !== "t1") throw new HttpError(404, "作業が見つかりません");
      calls.push(["cancel", id]);
    },
    cancelSession: (id: string) => void calls.push(["cancelSession", id]),
    get: () => undefined,
  },
  broker: {
    answer: (...args: unknown[]) => void calls.push(["answer", ...args]),
    pendingFor: () => [],
  },
  sessions: {
    list: async (limit: number) => (calls.push(["list", limit]), { workdirs: ["work"], sessions: [] }),
    messages: async (id: string, limit: number, offset: number) => (calls.push(["messages", id, limit, offset]), { session_id: id, workdir: "work", total: 0, offset, messages: [] }),
  },
  log: () => {},
});
let base = "";

before(async () => {
  await new Promise<void>((resolve) => server.listen(0, "127.0.0.1", resolve));
  base = `http://127.0.0.1:${(server.address() as AddressInfo).port}`;
});
after(() => server.close());

function call(path: string, init: RequestInit = {}, token = TOKEN) {
  return fetch(base + path, { ...init, headers: { authorization: `Bearer ${token}`, "content-type": "application/json", ...init.headers } });
}

test("トークンがなければ401、healthzは認証なし", async () => {
  assert.equal((await call("/sessions", {}, "wrong")).status, 401);
  assert.equal((await fetch(base + "/healthz")).status, 200);
});

test("作業の開始・承認の返事・中止", async () => {
  const started = await call("/tasks", { method: "POST", body: JSON.stringify({ input: "やって", session_id: "", workdir: "work" }) });
  assert.equal(started.status, 202);
  assert.deepEqual(await started.json(), { task_id: "t1", session_id: "s1", workdir: "work" });
  assert.equal((await call("/permissions/r1", { method: "POST", body: JSON.stringify({ allow: false }) })).status, 204);
  assert.equal((await call("/tasks/t1/cancel", { method: "POST" })).status, 204);
  assert.equal((await call("/tasks/t2/cancel", { method: "POST" })).status, 404);
  assert.equal((await call("/sessions/s1/cancel", { method: "POST" })).status, 204);
  assert.deepEqual(calls.splice(0), [
    ["start", "やって", "", "work"],
    ["answer", "r1", false, undefined],
    ["cancel", "t1"],
    ["cancelSession", "s1"],
  ]);
});

test("不正な本文とパラメータは400", async () => {
  assert.equal((await call("/tasks", { method: "POST", body: "[]" })).status, 400);
  assert.equal((await call("/tasks", { method: "POST", body: JSON.stringify({ input: 1 }) })).status, 400);
  assert.equal((await call("/permissions/r1", { method: "POST", body: JSON.stringify({ allow: "yes" }) })).status, 400);
  assert.equal((await call("/sessions?limit=0")).status, 400);
  assert.equal((await call("/sessions/%E0%A4%A/messages")).status, 400);
});

test("一覧と会話の取得", async () => {
  const list = await call("/sessions");
  assert.equal(list.status, 200);
  assert.deepEqual(await list.json(), { workdirs: ["work"], sessions: [] });
  assert.equal((await call("/sessions/abc/messages?limit=3&offset=4")).status, 200);
  assert.deepEqual(calls.splice(0), [
    ["list", 20],
    ["messages", "abc", 3, 4],
  ]);
});

test("contextの未知キーと入れ子を解釈せずに作業へ渡す", async () => {
  const context = { delivery: "bridgeには未知の値", future: { values: [1, null, { text: "日本語" }] } };
  const response = await call("/tasks", { method: "POST", body: JSON.stringify({ input: "実行", context }) });
  assert.equal(response.status, 202);
  assert.deepEqual(calls.splice(0), [["start", "実行", undefined, undefined, context]]);
});

test("contextはオブジェクトのみ受け付け、null、配列、プリミティブを拒否する", async () => {
  for (const context of [null, [], [1], "文字列", 1, true]) {
    const response = await call("/tasks", { method: "POST", body: JSON.stringify({ input: "実行", context }) });
    assert.equal(response.status, 400);
  }
  assert.deepEqual(calls.splice(0), []);
  assert.equal((await call("/tasks", { method: "POST", body: JSON.stringify({ input: "実行", context: {} }) })).status, 202);
  assert.deepEqual(calls.splice(0), [["start", "実行", undefined, undefined, {}]]);
});

test("contextの上限はUTF-8のJSONで16KiBとする", async () => {
  const overhead = Buffer.byteLength(JSON.stringify({ value: "" }), "utf8");
  const context = { value: "x".repeat(16 * 1024 - overhead) };
  assert.equal((await call("/tasks", { method: "POST", body: JSON.stringify({ input: "実行", context }) })).status, 202);
  assert.deepEqual(calls.splice(0), [["start", "実行", undefined, undefined, context]]);
  for (const value of [context.value + "x", "あ".repeat(6000)]) {
    assert.equal((await call("/tasks", { method: "POST", body: JSON.stringify({ input: "実行", context: { value } }) })).status, 400);
  }
  assert.deepEqual(calls.splice(0), []);
});
