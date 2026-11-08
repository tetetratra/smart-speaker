import assert from "node:assert/strict";
import { mkdirSync, mkdtempSync } from "node:fs";
import { tmpdir } from "node:os";
import { join } from "node:path";
import { test } from "node:test";
import { loadConfig } from "../src/config.ts";

const root = mkdtempSync(join(tmpdir(), "agent-bridge-config-"));
const work = join(root, "work");
const speaker = join(root, "speaker");
mkdirSync(work);
mkdirSync(speaker);

const base = { AGENT_BRIDGE_LISTEN: "127.0.0.1:8787", AGENT_BRIDGE_TOKEN: "0123456789abcdef", AGENT_BRIDGE_EVENTS_URL: "http://server/api/agent-events" };

test("AGENT_BRIDGE_WORKDIRS は 名前=パス をカンマ区切りで読み、先頭を既定にする", () => {
  const config = loadConfig({ ...base, AGENT_BRIDGE_WORKDIRS: ` work=${work} , speaker = ${speaker} ,` });
  assert.deepEqual(config.workdirs, [
    { name: "work", path: work },
    { name: "speaker", path: speaker },
  ]);
});

test("AGENT_BRIDGE_WORKDIRS の誤りは起動時に止める", () => {
  const load = (value: string) => () => loadConfig({ ...base, AGENT_BRIDGE_WORKDIRS: value });
  assert.throws(load(work), /名前=パス/);
  assert.throws(load("work=relative/path"), /絶対パス/);
  assert.throws(load(`work=${join(root, "missing")}`), /見つかりません/);
  assert.throws(load(`work=${work},work=${speaker}`), /名前が重複/);
  assert.throws(load(`work=${work},again=${work}/`), /同じ作業ディレクトリ/);
  assert.throws(load(","), /設定してください/);
});

test("以前の AGENT_BRIDGE_WORKDIR だけがあれば、新しい名前を案内する", () => {
  assert.throws(() => loadConfig({ ...base, AGENT_BRIDGE_WORKDIR: work }), /AGENT_BRIDGE_WORKDIRS/);
});
