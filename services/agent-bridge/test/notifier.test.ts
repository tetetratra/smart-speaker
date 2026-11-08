import assert from "node:assert/strict";
import { test } from "node:test";
import { Notifier } from "../src/notifier.ts";

const event = { type: "failed" as const, task_id: "t1", session_id: "s1", error: "x", request: "r" };

function notifier(statuses: (number | Error)[], now = () => 0) {
  const bodies: Record<string, unknown>[] = [];
  const headers: string[] = [];
  const fake = (async (_url: string, init: RequestInit) => {
    bodies.push(JSON.parse(String(init.body)));
    headers.push(String((init.headers as Record<string, string>).authorization));
    const status = statuses.shift()!;
    if (status instanceof Error) throw status;
    return new Response(null, { status });
  }) as typeof fetch;
  return { n: new Notifier({ url: "http://x", token: "tok", fetch: fake, sleep: async () => {}, now, log: () => {} }), bodies, headers };
}

test("失敗したら同じ event_id で再送する", async () => {
  const { n, bodies, headers } = notifier([new Error("down"), 503, 202]);
  assert.equal(await n.send(event, new Date(60_000)), true);
  assert.equal(bodies.length, 3);
  assert.ok(bodies[0].event_id);
  assert.ok(bodies.every((b) => b.event_id === bodies[0].event_id));
  assert.equal(headers[0], "Bearer tok");
});

test("4xxは再送しない", async () => {
  const { n, bodies } = notifier([401]);
  assert.equal(await n.send(event, new Date(60_000)), false);
  assert.equal(bodies.length, 1);
});

test("中止されたら再送しない", async () => {
  const { n, bodies } = notifier([503, 503, 503]);
  const abort = new AbortController();
  const sent = n.send(event, new Date(60_000), abort.signal);
  abort.abort();
  assert.equal(await sent, false);
  assert.equal(bodies.length, 1);
});

test("期限を過ぎたらあきらめる", async () => {
  const { n, bodies } = notifier([503, 503], () => 10_000);
  assert.equal(await n.send(event, new Date(5_000)), false);
  assert.equal(bodies.length, 1);
});
