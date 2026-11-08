import assert from "node:assert/strict";
import { test } from "node:test";
import type { EventInput } from "../src/protocol.ts";
import { PermissionBroker } from "../src/permissions.ts";

function setup(delivered = true, timeoutMs = 1000) {
  const events: EventInput[] = [];
  const broker = new PermissionBroker(async (event) => {
    events.push(event);
    return delivered;
  }, timeoutMs);
  return { broker, events };
}

function ask(signal = new AbortController().signal) {
  return { taskId: "t1", sessionId: "s1", tool: "Bash", input: { command: "ls" }, title: "Claude wants to run ls", signal };
}

test("返事で許可すると、元の入力のまま許可を返す", async () => {
  const { broker, events } = setup();
  const result = broker.request(ask());
  await Promise.resolve();
  const event = events[0];
  assert.equal(event.type, "permission_request");
  if (event.type !== "permission_request") return;
  assert.equal(event.title, "Claude wants to run ls");
  assert.deepEqual(broker.pendingFor("t1").map((p) => p.request_id), [event.request_id]);
  broker.answer(event.request_id, true);
  assert.deepEqual(await result, { behavior: "allow", updatedInput: { command: "ls" } });
  assert.deepEqual(broker.pendingFor("t1"), []);
});

test("拒否の返事にはメッセージが付く", async () => {
  const { broker, events } = setup();
  const result = broker.request(ask());
  await Promise.resolve();
  const event = events[0] as Extract<EventInput, { type: "permission_request" }>;
  broker.answer(event.request_id, false);
  assert.deepEqual(await result, { behavior: "deny", message: "ユーザーが拒否しました" });
});

test("未知のIDや答え済みのIDには404を返す", async () => {
  const { broker, events } = setup();
  const result = broker.request(ask());
  await Promise.resolve();
  const event = events[0] as Extract<EventInput, { type: "permission_request" }>;
  assert.throws(() => broker.answer("unknown", true), { status: 404 });
  broker.answer(event.request_id, false);
  await result;
  assert.throws(() => broker.answer(event.request_id, true), { status: 404 });
});

test("期限切れは拒否になる", async () => {
  const { broker } = setup(true, 10);
  const result = await broker.request(ask());
  assert.equal(result.behavior, "deny");
});

test("片付いた承認依頼の通知は再送を止める", async () => {
  const signals: (AbortSignal | undefined)[] = [];
  const broker = new PermissionBroker(async (_event, _deadline, signal) => (signals.push(signal), true), 1000);
  const result = broker.request(ask());
  broker.rejectTask("t1");
  await result;
  assert.equal(signals[0]?.aborted, true);
});

test("通知が届かなければ拒否になる", async () => {
  const { broker } = setup(false);
  const result = await broker.request(ask());
  assert.equal(result.behavior, "deny");
  assert.deepEqual(broker.pendingFor("t1"), []);
});

test("作業の中止と終了で、保留中の依頼は拒否になる", async () => {
  const { broker } = setup();
  const abort = new AbortController();
  const aborted = broker.request(ask(abort.signal));
  abort.abort();
  assert.equal((await aborted).behavior, "deny");

  const ended = broker.request(ask());
  broker.rejectTask("t1");
  assert.equal((await ended).behavior, "deny");
});
