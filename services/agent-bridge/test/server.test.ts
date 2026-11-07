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
    start: async (input: string, sessionId?: string) => (calls.push(["start", input, sessionId]), { task_id: "t1", session_id: "s1" }),
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
    list: async (limit: number) => (calls.push(["list", limit]), []),
    messages: async (id: string, limit: number, offset: number) => (calls.push(["messages", id, limit, offset]), { session_id: id, total: 0, offset, messages: [] }),
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
  const started = await call("/tasks", { method: "POST", body: JSON.stringify({ input: "やって", session_id: "" }) });
  assert.equal(started.status, 202);
  assert.deepEqual(await started.json(), { task_id: "t1", session_id: "s1" });
  assert.equal((await call("/permissions/r1", { method: "POST", body: JSON.stringify({ allow: false }) })).status, 204);
  assert.equal((await call("/tasks/t1/cancel", { method: "POST" })).status, 204);
  assert.equal((await call("/tasks/t2/cancel", { method: "POST" })).status, 404);
  assert.equal((await call("/sessions/s1/cancel", { method: "POST" })).status, 204);
  assert.deepEqual(calls.splice(0), [
    ["start", "やって", ""],
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
  assert.equal((await call("/sessions")).status, 200);
  assert.equal((await call("/sessions/abc/messages?limit=3&offset=4")).status, 200);
  assert.deepEqual(calls.splice(0), [
    ["list", 20],
    ["messages", "abc", 3, 4],
  ]);
});
